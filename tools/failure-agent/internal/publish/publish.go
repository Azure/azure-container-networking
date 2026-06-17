// Package publish writes the failure analysis back to the originating GitHub
// pull request as an idempotent comment, keyed by a hidden marker so reruns
// update the existing comment instead of spamming new ones.
package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Comment is a GitHub issue comment relevant to upserting.
type Comment struct {
	ID   int64
	Body string
}

// CommentStore is the minimal capability needed to upsert a PR comment.
type CommentStore interface {
	List(ctx context.Context) ([]Comment, error)
	Create(ctx context.Context, body string) (int64, error)
	Update(ctx context.Context, id int64, body string) error
}

// Upsert posts body to the PR, updating the existing comment that carries
// marker or creating a new one. It returns the action taken ("created" or
// "updated").
func Upsert(ctx context.Context, store CommentStore, marker, body string) (string, error) {
	if strings.TrimSpace(marker) == "" {
		return "", errors.New("marker is required for idempotent upsert")
	}

	comments, err := store.List(ctx)
	if err != nil {
		return "", fmt.Errorf("listing comments: %w", err)
	}
	for _, c := range comments {
		if strings.Contains(c.Body, marker) {
			if err := store.Update(ctx, c.ID, body); err != nil {
				return "", fmt.Errorf("updating comment %d: %w", c.ID, err)
			}
			return "updated", nil
		}
	}
	if _, err := store.Create(ctx, body); err != nil {
		return "", fmt.Errorf("creating comment: %w", err)
	}
	return "created", nil
}

// ParseRepo splits an "owner/repo" string (e.g. BUILD_REPOSITORY_NAME).
func ParseRepo(full string) (owner, repo string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(full), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
