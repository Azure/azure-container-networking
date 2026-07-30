package prsync

import (
	"context"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/publish"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/store"
)

type fakeReader struct {
	pr  publish.PullRequest
	err error
}

func (f fakeReader) FetchPullRequest(context.Context) (publish.PullRequest, error) {
	return f.pr, f.err
}

type fakeUpdater struct {
	link   store.PRLink
	status store.IncidentStatus
}

func (f *fakeUpdater) UpsertPRLink(_ context.Context, link store.PRLink) error {
	f.link = link
	return nil
}

func (f *fakeUpdater) UpdateStatus(_ context.Context, _ string, status store.IncidentStatus) error {
	f.status = status
	return nil
}

func TestSyncMapsGitHubState(t *testing.T) {
	cases := []struct {
		name       string
		pr         publish.PullRequest
		wantStatus store.IncidentStatus
		wantLink   string
	}{
		{"open", publish.PullRequest{Number: 1, State: "open"}, store.StatusPROpen, "open"},
		{"merged", publish.PullRequest{Number: 2, State: "closed", Merged: true}, store.StatusMerged, "merged"},
		{"closed unmerged", publish.PullRequest{Number: 3, State: "closed"}, store.StatusInvestigating, "closed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			up := &fakeUpdater{}
			got, err := Sync(context.Background(), fakeReader{pr: tc.pr}, up, "inc-1")
			if err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if got != tc.wantStatus {
				t.Errorf("status: got %s, want %s", got, tc.wantStatus)
			}
			if up.status != tc.wantStatus {
				t.Errorf("persisted status: got %s, want %s", up.status, tc.wantStatus)
			}
			if up.link.State != tc.wantLink {
				t.Errorf("link state: got %s, want %s", up.link.State, tc.wantLink)
			}
			if up.link.Number != tc.pr.Number {
				t.Errorf("link number: got %d, want %d", up.link.Number, tc.pr.Number)
			}
		})
	}
}

func TestSyncReturnsReaderError(t *testing.T) {
	up := &fakeUpdater{}
	_, err := Sync(context.Background(), fakeReader{err: context.DeadlineExceeded}, up, "inc-1")
	if err == nil {
		t.Fatal("expected error when reader fails")
	}
	if up.status != "" {
		t.Error("status must not change when the reader fails")
	}
}
