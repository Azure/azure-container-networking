// Package prsync reconciles an incident's lifecycle with the authoritative pull
// request state reported by GitHub. It is deliberately separate from the PR
// comment upsert path: comments inform humans, this records ground-truth state.
package prsync

import (
	"context"
	"fmt"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/publish"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/store"
)

// Reader returns the authoritative pull request state.
type Reader interface {
	FetchPullRequest(ctx context.Context) (publish.PullRequest, error)
}

// Updater persists the reconciled state. The knowledge store satisfies it.
type Updater interface {
	UpsertPRLink(ctx context.Context, link store.PRLink) error
	UpdateStatus(ctx context.Context, incidentID string, status store.IncidentStatus) error
}

// Sync reads the PR's GitHub state, records the link, and transitions the
// incident's lifecycle accordingly. It returns the new lifecycle status.
func Sync(ctx context.Context, reader Reader, updater Updater, incidentID string) (store.IncidentStatus, error) {
	pr, err := reader.FetchPullRequest(ctx)
	if err != nil {
		return "", fmt.Errorf("fetching pull request: %w", err)
	}

	status, linkState := mapState(pr)
	if err := updater.UpsertPRLink(ctx, store.PRLink{
		IncidentID: incidentID,
		Number:     pr.Number,
		URL:        pr.URL,
		State:      linkState,
	}); err != nil {
		return "", err
	}
	if err := updater.UpdateStatus(ctx, incidentID, status); err != nil {
		return "", err
	}
	return status, nil
}

// mapState maps GitHub's (state, merged) into the incident lifecycle and the
// stored PR link state. A merged PR is the strongest signal; a closed-unmerged
// PR was abandoned, so the incident returns to investigation.
func mapState(pr publish.PullRequest) (store.IncidentStatus, string) {
	switch {
	case pr.Merged:
		return store.StatusMerged, "merged"
	case pr.State == "closed":
		return store.StatusInvestigating, "closed"
	default:
		return store.StatusPROpen, "open"
	}
}
