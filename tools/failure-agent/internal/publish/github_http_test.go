package publish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListHandlesLeadingWhitespaceArray verifies that a GitHub response with
// leading whitespace before a JSON array is decoded correctly after the
// TrimSpace fix in doComments.
func TestListHandlesLeadingWhitespaceArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulate response with leading newlines/spaces before the array.
		w.Write([]byte("\n  [{\"id\":1,\"body\":\"hello\"}]\n"))
	}))
	defer srv.Close()

	store, err := NewGitHubCommentStore(GitHubConfig{
		BaseURL:    srv.URL,
		Token:      "test-token",
		Owner:      "Azure",
		Repo:       "acn",
		PRNumber:   42,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewGitHubCommentStore: %v", err)
	}

	comments, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "hello" {
		t.Errorf("body: got %q, want %q", comments[0].Body, "hello")
	}
}

// TestCreateHandlesLeadingWhitespaceSingleObject verifies that a GitHub
// response with leading whitespace before a single JSON object is decoded
// correctly.
func TestCreateHandlesLeadingWhitespaceSingleObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulate response with leading whitespace before a single object.
		w.Write([]byte("  \n{\"id\":99,\"body\":\"created\"}\n"))
	}))
	defer srv.Close()

	store, err := NewGitHubCommentStore(GitHubConfig{
		BaseURL:    srv.URL,
		Token:      "test-token",
		Owner:      "Azure",
		Repo:       "acn",
		PRNumber:   42,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewGitHubCommentStore: %v", err)
	}

	id, err := store.Create(context.Background(), "new comment")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id != 99 {
		t.Errorf("id: got %d, want 99", id)
	}
}

// TestListHandlesEmptyWhitespaceResponse verifies that a response containing
// only whitespace is treated as empty (no comments).
func TestListHandlesEmptyWhitespaceResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("   \n\t  \n"))
	}))
	defer srv.Close()

	store, err := NewGitHubCommentStore(GitHubConfig{
		BaseURL:    srv.URL,
		Token:      "test-token",
		Owner:      "Azure",
		Repo:       "acn",
		PRNumber:   42,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewGitHubCommentStore: %v", err)
	}

	comments, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments, got %d", len(comments))
	}
}
