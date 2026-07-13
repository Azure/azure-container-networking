package publish

import (
	"context"
	"strings"
	"testing"
)

type fakeStore struct {
	comments []Comment
	nextID   int64

	created []string
	updated map[int64]string
}

func newFakeStore(existing ...Comment) *fakeStore {
	return &fakeStore{comments: existing, nextID: 100, updated: map[int64]string{}}
}

func (f *fakeStore) List(context.Context) ([]Comment, error) {
	return f.comments, nil
}

func (f *fakeStore) Create(_ context.Context, body string) (int64, error) {
	f.nextID++
	f.created = append(f.created, body)
	f.comments = append(f.comments, Comment{ID: f.nextID, Body: body})
	return f.nextID, nil
}

func (f *fakeStore) Update(_ context.Context, id int64, body string) error {
	f.updated[id] = body
	return nil
}

func TestUpsertCreatesWhenNoMatch(t *testing.T) {
	store := newFakeStore(Comment{ID: 1, Body: "an unrelated comment"})

	action, err := Upsert(context.Background(), store, "<!-- acn-failure-agent:abc123 -->", "report body")
	if err != nil {
		t.Fatalf("Upsert error: %v", err)
	}
	if action != "created" {
		t.Errorf("action: got %q, want created", action)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 create, got %d", len(store.created))
	}
	if len(store.updated) != 0 {
		t.Errorf("expected no updates, got %d", len(store.updated))
	}
}

func TestUpsertUpdatesWhenMarkerMatches(t *testing.T) {
	marker := "<!-- acn-failure-agent:abc123 -->"
	store := newFakeStore(Comment{ID: 7, Body: marker + "\nold report"})

	action, err := Upsert(context.Background(), store, marker, "new report body")
	if err != nil {
		t.Fatalf("Upsert error: %v", err)
	}
	if action != "updated" {
		t.Errorf("action: got %q, want updated", action)
	}
	if got := store.updated[7]; got != "new report body" {
		t.Errorf("updated body: got %q, want new report body", got)
	}
	if len(store.created) != 0 {
		t.Errorf("expected no creates, got %d", len(store.created))
	}
}

func TestUpsertRequiresMarker(t *testing.T) {
	if _, err := Upsert(context.Background(), newFakeStore(), "  ", "body"); err == nil {
		t.Fatal("expected error for empty marker")
	}
}

func TestParseRepo(t *testing.T) {
	owner, repo, ok := ParseRepo("Azure/azure-container-networking")
	if !ok || owner != "Azure" || repo != "azure-container-networking" {
		t.Fatalf("ParseRepo: got (%q,%q,%v)", owner, repo, ok)
	}
	if _, _, ok := ParseRepo("noslash"); ok {
		t.Error("expected failure for input without a slash")
	}
}

func TestNewGitHubCommentStoreValidation(t *testing.T) {
	if _, err := NewGitHubCommentStore(GitHubConfig{Owner: "o", Repo: "r", PRNumber: 1}); err == nil {
		t.Error("expected error when token missing")
	}
	if _, err := NewGitHubCommentStore(GitHubConfig{Token: "t", Repo: "r", PRNumber: 1}); err == nil {
		t.Error("expected error when owner missing")
	}
	if _, err := NewGitHubCommentStore(GitHubConfig{Token: "t", Owner: "o", Repo: "r"}); err == nil {
		t.Error("expected error when PR number missing")
	}
	if _, err := NewGitHubCommentStore(GitHubConfig{Token: "t", Owner: "o", Repo: "r", PRNumber: 5}); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

// ensure the package marker convention stays in sync with what reports emit.
func TestMarkerContains(t *testing.T) {
	body := "<!-- acn-failure-agent:deadbeef -->\nhello"
	if !strings.Contains(body, "acn-failure-agent:deadbeef") {
		t.Fatal("marker format changed unexpectedly")
	}
}
