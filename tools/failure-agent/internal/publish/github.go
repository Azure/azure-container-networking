package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	apiVersion     = "2022-11-28"
	pageSize       = 100
	requestTimeout = 30 * time.Second
)

// GitHubConfig configures a GitHubCommentStore.
type GitHubConfig struct {
	BaseURL    string // defaults to https://api.github.com
	Token      string
	Owner      string
	Repo       string
	PRNumber   int
	HTTPClient *http.Client
}

// GitHubCommentStore implements CommentStore against the GitHub REST API for a
// single pull request's issue comments.
type GitHubCommentStore struct {
	httpClient *http.Client
	baseURL    string
	token      string
	owner      string
	repo       string
	prNumber   int
}

// NewGitHubCommentStore validates cfg and returns a ready store.
func NewGitHubCommentStore(cfg GitHubConfig) (*GitHubCommentStore, error) {
	if cfg.Token == "" {
		return nil, errors.New("github token is required")
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		return nil, errors.New("github owner and repo are required")
	}
	if cfg.PRNumber <= 0 {
		return nil, errors.New("a valid pull request number is required")
	}

	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: requestTimeout}
	}
	return &GitHubCommentStore{
		httpClient: hc,
		baseURL:    base,
		token:      cfg.Token,
		owner:      cfg.Owner,
		repo:       cfg.Repo,
		prNumber:   cfg.PRNumber,
	}, nil
}

type ghComment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// PullRequest is the subset of a GitHub pull request the agent uses to sync an
// incident's lifecycle. State and Merged are authoritative from GitHub.
type PullRequest struct {
	Number int    `json:"number"`
	State  string `json:"state"` // "open" or "closed"
	Merged bool   `json:"merged"`
	URL    string `json:"html_url"`
}

// FetchPullRequest reads the authoritative pull request state from GitHub. It is
// independent of the comment upsert path.
func (g *GitHubCommentStore) FetchPullRequest(ctx context.Context) (PullRequest, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", g.baseURL, g.owner, g.repo, g.prNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return PullRequest{}, err
	}
	g.setHeaders(req)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return PullRequest{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return PullRequest{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PullRequest{}, fmt.Errorf("github api %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var pr PullRequest
	if err := json.Unmarshal(data, &pr); err != nil {
		return PullRequest{}, fmt.Errorf("decoding pull request: %w", err)
	}
	return pr, nil
}

// List returns all issue comments on the pull request, following pagination.
func (g *GitHubCommentStore) List(ctx context.Context) ([]Comment, error) {
	var out []Comment
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=%d&page=%d",
			g.baseURL, g.owner, g.repo, g.prNumber, pageSize, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		g.setHeaders(req)

		batch, err := g.doComments(req)
		if err != nil {
			return nil, err
		}
		for _, c := range batch {
			out = append(out, Comment{ID: c.ID, Body: c.Body})
		}
		if len(batch) < pageSize {
			return out, nil
		}
	}
}

// Create posts a new issue comment and returns its ID.
func (g *GitHubCommentStore) Create(ctx context.Context, body string) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", g.baseURL, g.owner, g.repo, g.prNumber)
	req, err := g.newBodyRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return 0, err
	}
	created, err := g.doComments(req)
	if err != nil {
		return 0, err
	}
	if len(created) == 0 {
		return 0, errors.New("github did not return the created comment")
	}
	return created[0].ID, nil
}

// Update edits an existing issue comment by ID.
func (g *GitHubCommentStore) Update(ctx context.Context, id int64, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", g.baseURL, g.owner, g.repo, id)
	req, err := g.newBodyRequest(ctx, http.MethodPatch, url, body)
	if err != nil {
		return err
	}
	_, err = g.doComments(req)
	return err
}

func (g *GitHubCommentStore) newBodyRequest(ctx context.Context, method, url, body string) (*http.Request, error) {
	payload, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	g.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (g *GitHubCommentStore) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
}

// doComments executes the request and decodes the JSON body, which the GitHub
// comments endpoints return either as a single object or an array.
func (g *GitHubCommentStore) doComments(req *http.Request) ([]ghComment, error) {
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github api %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	if data[0] == '[' {
		var arr []ghComment
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, fmt.Errorf("decoding github response: %w", err)
		}
		return arr, nil
	}
	var single ghComment
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("decoding github response: %w", err)
	}
	return []ghComment{single}, nil
}
