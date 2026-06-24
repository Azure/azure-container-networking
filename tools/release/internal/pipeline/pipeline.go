package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	doer httpDoer
}

// WaitOptions configures how to find and wait for an auto-triggered pipeline run.
type WaitOptions struct {
	Org          string        `json:"org"`
	Project      string        `json:"project"`
	DefinitionID string        `json:"definition_id"`
	Token        string        `json:"-"` // Bearer token (from OIDC)
	Tag          string        `json:"tag"`
	Timeout      time.Duration `json:"timeout"`
	Name         string        `json:"name"`
}

type WaitResult struct {
	RunURL string `json:"run_url"`
	RunID  int    `json:"run_id"`
	Result string `json:"result"`
}

type buildListResponse struct {
	Value []buildEntry `json:"value"`
}

type buildEntry struct {
	ID          int    `json:"id"`
	Status      string `json:"status"`
	Result      string `json:"result"`
	SourceBranch string `json:"sourceBranch"`
}

func NewClient() *Client {
	return &Client{doer: http.DefaultClient}
}

// WaitForRun discovers a pipeline run triggered by the given tag and waits for it to complete.
// It polls the ADO builds API until it finds a run matching the tag, then polls until completion.
func (c *Client) WaitForRun(ctx context.Context, opts WaitOptions, progress io.Writer) (WaitResult, error) {
	if progress == nil {
		progress = io.Discard
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "ADO pipeline"
	}

	pollCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Phase 1: Find the run triggered by the tag
	fmt.Fprintf(progress, "%s: waiting for run triggered by tag %s...\n", name, opts.Tag)
	runID, err := c.findRunByTag(pollCtx, opts, progress)
	if err != nil {
		return WaitResult{}, fmt.Errorf("%s: %w", name, err)
	}

	runURL := buildRunURL(opts, runID)
	fmt.Fprintf(progress, "%s: found run %d: %s\n", name, runID, runURL)

	// Phase 2: Wait for the run to complete
	result, err := c.waitForCompletion(pollCtx, opts, runID, progress)
	if err != nil {
		return WaitResult{}, fmt.Errorf("%s: %w", name, err)
	}

	return WaitResult{
		RunURL: runURL,
		RunID:  runID,
		Result: result,
	}, nil
}

// findRunByTag polls the builds list until a run matching refs/tags/<tag> appears.
func (c *Client) findRunByTag(ctx context.Context, opts WaitOptions, progress io.Writer) (int, error) {
	expectedBranch := "refs/tags/" + opts.Tag
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		builds, err := c.listBuilds(ctx, opts)
		if err != nil {
			fmt.Fprintf(progress, "%s: error listing builds: %v (retrying...)\n", opts.Name, err)
		} else {
			for _, b := range builds {
				if strings.EqualFold(b.SourceBranch, expectedBranch) {
					return b.ID, nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("timed out waiting for run triggered by %s", opts.Tag)
		case <-ticker.C:
			fmt.Fprintf(progress, "%s: no run found yet for %s, polling...\n", opts.Name, opts.Tag)
		}
	}
}

func (c *Client) listBuilds(ctx context.Context, opts WaitOptions) ([]buildEntry, error) {
	url := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/build/builds?definitions=%s&$top=20&api-version=7.0",
		opts.Org, opts.Project, opts.DefinitionID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+opts.Token)

	body, err := c.do(req)
	if err != nil {
		return nil, err
	}

	var resp buildListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding builds list: %w", err)
	}

	return resp.Value, nil
}

func (c *Client) waitForCompletion(ctx context.Context, opts WaitOptions, runID int, progress io.Writer) (string, error) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		state, err := c.buildStatus(ctx, opts, runID)
		if err != nil {
			return "", err
		}

		if strings.EqualFold(state.Status, "completed") {
			if strings.EqualFold(state.Result, "succeeded") {
				fmt.Fprintf(progress, "%s: succeeded\n", opts.Name)
				return "succeeded", nil
			}
			return state.Result, fmt.Errorf("completed with result %q", state.Result)
		}

		fmt.Fprintf(progress, "%s: status=%s result=%s\n", opts.Name, state.Status, state.Result)

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out after %s: %w", opts.Timeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

type buildStatusResponse struct {
	Status string `json:"status"`
	Result string `json:"result"`
}

func (c *Client) buildStatus(ctx context.Context, opts WaitOptions, runID int) (buildStatusResponse, error) {
	url := fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/build/builds/%d?api-version=7.0",
		opts.Org, opts.Project, runID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return buildStatusResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+opts.Token)

	body, err := c.do(req)
	if err != nil {
		return buildStatusResponse{}, fmt.Errorf("checking status: %w", err)
	}

	var resp buildStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return buildStatusResponse{}, fmt.Errorf("decoding status: %w", err)
	}

	return resp, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func buildRunURL(opts WaitOptions, runID int) string {
	return fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_build/results?buildId=%d&view=results",
		opts.Org, opts.Project, runID,
	)
}
