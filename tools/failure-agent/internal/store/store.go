// Package store is the SQL-first knowledge base for the failure agent. It records
// every incident, its fingerprint, lifecycle events, and linked pull requests so
// the agent can recognize recurring failures, skip duplicate work, draw on prior
// resolutions, and surface pipeline flakiness trends across runs.
//
// The store is driver-agnostic: it talks to *sql.DB. Tests and the demo use the
// pure-Go modernc.org/sqlite driver; production can point at any SQL backend.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// IncidentStatus is the lifecycle state of an incident.
type IncidentStatus string

const (
	StatusNew               IncidentStatus = "new"
	StatusInvestigating     IncidentStatus = "investigating"
	StatusPROpen            IncidentStatus = "pr_open"
	StatusMerged            IncidentStatus = "merged"
	StatusValidatedResolved IncidentStatus = "validated_resolved"
	StatusAnalysisFailed    IncidentStatus = "analysis_failed"
	StatusDuplicateSkipped  IncidentStatus = "duplicate_skipped"
)

// activeStatuses are incidents representing in-flight, unresolved work. A new
// occurrence of the same fingerprint while one of these exists is a duplicate.
var activeStatuses = map[IncidentStatus]struct{}{
	StatusNew:           {},
	StatusInvestigating: {},
	StatusPROpen:        {},
	StatusMerged:        {},
}

// Incident is a persisted analysis record.
type Incident struct {
	ID          string
	Fingerprint string
	Pipeline    string
	Category    string
	Confidence  float64
	Summary     string
	ProposedFix string
	Status      IncidentStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PRLink ties an incident to a pull request and its GitHub-reported state.
type PRLink struct {
	IncidentID string
	Number     int
	URL        string
	State      string // open, merged, closed (authoritative from GitHub)
	UpdatedAt  time.Time
}

// Store persists and queries the knowledge base.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open returns a Store backed by the given driver and DSN, running migrations.
func Open(ctx context.Context, driver, dsn string) (*Store, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening knowledge store: %w", err)
	}
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS fingerprints (
	hash              TEXT PRIMARY KEY,
	normalized_signal TEXT NOT NULL DEFAULT '',
	first_seen        TIMESTAMP NOT NULL,
	last_seen         TIMESTAMP NOT NULL,
	occurrence_count  INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS incidents (
	id           TEXT PRIMARY KEY,
	fingerprint  TEXT NOT NULL,
	pipeline     TEXT NOT NULL DEFAULT '',
	category     TEXT NOT NULL DEFAULT '',
	confidence   REAL NOT NULL DEFAULT 0,
	summary      TEXT NOT NULL DEFAULT '',
	proposed_fix TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL,
	created_at   TIMESTAMP NOT NULL,
	updated_at   TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_incidents_fingerprint ON incidents(fingerprint);
CREATE TABLE IF NOT EXISTS incident_fingerprints (
	incident_id TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	PRIMARY KEY (incident_id, fingerprint)
);
CREATE TABLE IF NOT EXISTS pr_links (
	incident_id TEXT NOT NULL,
	number      INTEGER NOT NULL,
	url         TEXT NOT NULL DEFAULT '',
	state       TEXT NOT NULL DEFAULT '',
	updated_at  TIMESTAMP NOT NULL,
	PRIMARY KEY (incident_id, number)
);
CREATE TABLE IF NOT EXISTS incident_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	incident_id TEXT NOT NULL,
	name        TEXT NOT NULL,
	detail      TEXT NOT NULL DEFAULT '',
	created_at  TIMESTAMP NOT NULL
);`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrating knowledge store: %w", err)
	}
	return nil
}

// CreateIncident inserts a new incident, records its fingerprint occurrence, and
// links the two. It returns the incident ID, generating one when empty.
func (s *Store) CreateIncident(ctx context.Context, inc Incident) (string, error) {
	if inc.Fingerprint == "" {
		return "", errors.New("incident fingerprint is required")
	}
	if inc.Status == "" {
		return "", errors.New("incident status is required")
	}
	if inc.ID == "" {
		inc.ID = uuid.NewString()
	}
	now := s.now().UTC()
	inc.CreatedAt, inc.UpdatedAt = now, now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO fingerprints (hash, normalized_signal, first_seen, last_seen, occurrence_count)
VALUES (?, '', ?, ?, 1)
ON CONFLICT(hash) DO UPDATE SET last_seen = excluded.last_seen, occurrence_count = occurrence_count + 1`,
		inc.Fingerprint, now, now); err != nil {
		return "", fmt.Errorf("recording fingerprint: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO incidents (id, fingerprint, pipeline, category, confidence, summary, proposed_fix, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inc.ID, inc.Fingerprint, inc.Pipeline, inc.Category, inc.Confidence, inc.Summary, inc.ProposedFix, string(inc.Status), now, now); err != nil {
		return "", fmt.Errorf("inserting incident: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO incident_fingerprints (incident_id, fingerprint) VALUES (?, ?)`,
		inc.ID, inc.Fingerprint); err != nil {
		return "", fmt.Errorf("linking fingerprint: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing incident: %w", err)
	}
	return inc.ID, nil
}

// UpdateStatus transitions an incident to a new lifecycle state.
func (s *Store) UpdateStatus(ctx context.Context, incidentID string, status IncidentStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE incidents SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), s.now().UTC(), incidentID)
	if err != nil {
		return fmt.Errorf("updating incident status: %w", err)
	}
	return requireRow(res, incidentID)
}

// AppendEvent records a lifecycle event for an incident.
func (s *Store) AppendEvent(ctx context.Context, incidentID, name, detail string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO incident_events (incident_id, name, detail, created_at) VALUES (?, ?, ?, ?)`,
		incidentID, name, detail, s.now().UTC()); err != nil {
		return fmt.Errorf("appending incident event: %w", err)
	}
	return nil
}

// ActiveByFingerprint returns the most recent unresolved incident for a
// fingerprint, or nil when none is active.
func (s *Store) ActiveByFingerprint(ctx context.Context, fingerprint string) (*Incident, error) {
	rows, err := s.db.QueryContext(ctx, selectIncident+` WHERE fingerprint = ? ORDER BY created_at DESC`, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("querying active incident: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		if _, active := activeStatuses[inc.Status]; active {
			return &inc, nil
		}
	}
	return nil, rows.Err()
}

// PriorByFingerprint returns earlier incidents for a fingerprint, split into
// validated resolutions and still-unresolved occurrences, excluding excludeID.
// Each slice is capped at limit, newest first.
func (s *Store) PriorByFingerprint(ctx context.Context, fingerprint, excludeID string, limit int) (resolved, unresolved []Incident, err error) {
	rows, err := s.db.QueryContext(ctx, selectIncident+` WHERE fingerprint = ? AND id != ? ORDER BY created_at DESC`, fingerprint, excludeID)
	if err != nil {
		return nil, nil, fmt.Errorf("querying prior incidents: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, nil, err
		}
		switch inc.Status {
		case StatusValidatedResolved:
			if len(resolved) < limit {
				resolved = append(resolved, inc)
			}
		default:
			if _, active := activeStatuses[inc.Status]; active && len(unresolved) < limit {
				unresolved = append(unresolved, inc)
			}
		}
	}
	return resolved, unresolved, rows.Err()
}

// UpsertPRLink records or updates the pull request tied to an incident.
func (s *Store) UpsertPRLink(ctx context.Context, link PRLink) error {
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO pr_links (incident_id, number, url, state, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(incident_id, number) DO UPDATE SET url = excluded.url, state = excluded.state, updated_at = excluded.updated_at`,
		link.IncidentID, link.Number, link.URL, link.State, s.now().UTC()); err != nil {
		return fmt.Errorf("upserting pr link: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

// selectIncident is the column list shared by incident queries.
const selectIncident = `SELECT id, fingerprint, pipeline, category, confidence, summary, proposed_fix, status, created_at, updated_at FROM incidents`

func scanIncident(r rowScanner) (Incident, error) {
	var inc Incident
	var status string
	if err := r.Scan(&inc.ID, &inc.Fingerprint, &inc.Pipeline, &inc.Category,
		&inc.Confidence, &inc.Summary, &inc.ProposedFix, &status, &inc.CreatedAt, &inc.UpdatedAt); err != nil {
		return Incident{}, fmt.Errorf("scanning incident: %w", err)
	}
	inc.Status = IncidentStatus(status)
	return inc, nil
}

func requireRow(res sql.Result, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("incident %q not found", id)
	}
	return nil
}
