package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "knowledge.db")
	s, err := Open(context.Background(), "sqlite", dsn)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Deterministic, monotonically increasing clock for stable ordering.
	base := time.Unix(1_700_000_000, 0).UTC()
	tick := 0
	s.now = func() time.Time {
		tick++
		return base.Add(time.Duration(tick) * time.Second)
	}
	return s
}

func TestCreateIncidentRecordsFingerprintOccurrence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id1, err := s.CreateIncident(ctx, Incident{Fingerprint: "abc", Pipeline: "ACN PR", Status: StatusNew})
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	if id1 == "" {
		t.Fatal("expected generated incident id")
	}
	if _, err := s.CreateIncident(ctx, Incident{Fingerprint: "abc", Status: StatusNew}); err != nil {
		t.Fatalf("create 2: %v", err)
	}

	var count int
	if err := s.db.QueryRow(`SELECT occurrence_count FROM fingerprints WHERE hash = ?`, "abc").Scan(&count); err != nil {
		t.Fatalf("reading occurrence: %v", err)
	}
	if count != 2 {
		t.Errorf("occurrence_count: got %d, want 2", count)
	}
}

func TestActiveByFingerprintFindsUnresolved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIncident(ctx, Incident{Fingerprint: "fp1", Status: StatusNew})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateStatus(ctx, id, StatusPROpen); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.ActiveByFingerprint(ctx, "fp1")
	if err != nil {
		t.Fatalf("active lookup: %v", err)
	}
	if got == nil || got.ID != id {
		t.Fatalf("expected active incident %s, got %+v", id, got)
	}
	if got.Status != StatusPROpen {
		t.Errorf("status: got %s, want pr_open", got.Status)
	}
}

func TestActiveByFingerprintNilWhenResolved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.CreateIncident(ctx, Incident{Fingerprint: "fp2", Status: StatusNew})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.UpdateStatus(ctx, id, StatusValidatedResolved); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.ActiveByFingerprint(ctx, "fp2")
	if err != nil {
		t.Fatalf("active lookup: %v", err)
	}
	if got != nil {
		t.Errorf("expected no active incident, got %+v", got)
	}
}

func TestActiveByFingerprintReturnsLatest(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, _ := s.CreateIncident(ctx, Incident{Fingerprint: "fp3", Status: StatusNew})
	_ = s.UpdateStatus(ctx, first, StatusValidatedResolved)
	second, _ := s.CreateIncident(ctx, Incident{Fingerprint: "fp3", Status: StatusNew})

	got, err := s.ActiveByFingerprint(ctx, "fp3")
	if err != nil {
		t.Fatalf("active lookup: %v", err)
	}
	if got == nil || got.ID != second {
		t.Fatalf("expected latest active incident %s, got %+v", second, got)
	}
}

func TestPriorByFingerprintSplitsResolvedAndUnresolved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	resolvedID, _ := s.CreateIncident(ctx, Incident{Fingerprint: "fp", Summary: "old", ProposedFix: "do the thing", Status: StatusNew})
	_ = s.UpdateStatus(ctx, resolvedID, StatusValidatedResolved)

	openID, _ := s.CreateIncident(ctx, Incident{Fingerprint: "fp", Summary: "in flight", Status: StatusNew})
	_ = s.UpdateStatus(ctx, openID, StatusPROpen)

	current, _ := s.CreateIncident(ctx, Incident{Fingerprint: "fp", Status: StatusNew})

	resolved, unresolved, err := s.PriorByFingerprint(ctx, "fp", current, 10)
	if err != nil {
		t.Fatalf("prior: %v", err)
	}
	if len(resolved) != 1 || resolved[0].ID != resolvedID {
		t.Fatalf("resolved: got %+v, want %s", resolved, resolvedID)
	}
	if resolved[0].ProposedFix != "do the thing" {
		t.Errorf("proposed fix not persisted: %q", resolved[0].ProposedFix)
	}
	if len(unresolved) != 1 || unresolved[0].ID != openID {
		t.Fatalf("unresolved: got %+v, want %s", unresolved, openID)
	}
}

func TestUpdateStatusUnknownIncident(t *testing.T) {
	s := newTestStore(t)
	if err := s.UpdateStatus(context.Background(), "missing", StatusMerged); err == nil {
		t.Error("expected error updating unknown incident")
	}
}

func TestAppendEventAndPRLink(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, _ := s.CreateIncident(ctx, Incident{Fingerprint: "fp4", Status: StatusNew})
	if err := s.AppendEvent(ctx, id, "duplicate_skipped", "active PR #12"); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := s.UpsertPRLink(ctx, PRLink{IncidentID: id, Number: 12, URL: "https://x/12", State: "open"}); err != nil {
		t.Fatalf("upsert pr link: %v", err)
	}
	if err := s.UpsertPRLink(ctx, PRLink{IncidentID: id, Number: 12, State: "merged"}); err != nil {
		t.Fatalf("update pr link: %v", err)
	}

	var state string
	if err := s.db.QueryRow(`SELECT state FROM pr_links WHERE incident_id = ? AND number = 12`, id).Scan(&state); err != nil {
		t.Fatalf("reading pr link: %v", err)
	}
	if state != "merged" {
		t.Errorf("pr state: got %s, want merged", state)
	}

	var events int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM incident_events WHERE incident_id = ?`, id).Scan(&events); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if events != 1 {
		t.Errorf("events: got %d, want 1", events)
	}
}
