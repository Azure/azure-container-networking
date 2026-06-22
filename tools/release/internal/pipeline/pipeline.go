package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	retry "github.com/avast/retry-go/v4"
)

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Client struct {
	doer httpDoer
}

type RunOptions struct {
	Org        string        `json:"org"`
	Project    string        `json:"project"`
	PipelineID string        `json:"pipeline_id"`
	PAT        string        `json:"-"`
	Tag        string        `json:"tag"`
	MaxRetries uint          `json:"max_retries"`
	Timeout    time.Duration `json:"timeout"`
	Name       string        `json:"name"`
}

type RunResult struct {
	RunURL string `json:"run_url"`
}

type triggerResponse struct {
	ID int `json:"id"`
}

type buildStatusResponse struct {
	Status string `json:"status"`
	Result string `json:"result"`
}

func NewClient() *Client {
	return &Client{doer: http.DefaultClient}
}

func (c *Client) Run(ctx context.Context, opts RunOptions, progress io.Writer) (RunResult, error) {
	if progress == nil {
		progress = io.Discard
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "ADO pipeline"
	}

	var result RunResult
	attempt := 0
	err := retry.Do(
		func() error {
			attempt++
			fmt.Fprintf(progress, "%s attempt %d/%d\n", name, attempt, opts.MaxRetries)

			runID, err := c.triggerRun(ctx, opts)
			if err != nil {
				return err
			}

			runURL := buildRunURL(opts, runID)
			fmt.Fprintf(progress, "triggered %s: %s\n", name, runURL)

			pollCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
			defer cancel()

			if err := c.waitForCompletion(pollCtx, opts, runID, progress); err != nil {
				// Cancel the ADO run before retrying to avoid orphaned parallel runs
				fmt.Fprintf(progress, "cancelling run %d before retry...\n", runID)
				if cancelErr := c.cancelRun(ctx, opts, runID); cancelErr != nil {
					fmt.Fprintf(progress, "warning: failed to cancel run %d: %v\n", runID, cancelErr)
				}
				return err
			}

			result = RunResult{RunURL: runURL}
			return nil
		},
		retry.Attempts(opts.MaxRetries),
		retry.Context(ctx),
		retry.Delay(time.Minute),
		retry.DelayType(retry.FixedDelay),
		retry.LastErrorOnly(true),
	)
	if err != nil {
		return RunResult{}, fmt.Errorf("%s exhausted %d attempt(s): %w", name, opts.MaxRetries, err)
	}

	return result, nil
}

func (c *Client) triggerRun(ctx context.Context, opts RunOptions) (int, error) {
	payload := map[string]any{
		"resources": map[string]any{
			"repositories": map[string]any{
				"self": map[string]any{
					"refName": "refs/tags/" + opts.Tag,
				},
			},
		},
		"templateParameters": map[string]any{
			"tag": opts.Tag,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshaling trigger payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, triggerURL(opts), bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating trigger request: %w", err)
	}
	req.Header.Set("Authorization", basicAuthPAT(opts.PAT))
	req.Header.Set("Content-Type", "application/json")

	respBody, err := c.do(req)
	if err != nil {
		return 0, fmt.Errorf("triggering pipeline: %w", err)
	}

	var resp triggerResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return 0, fmt.Errorf("decoding trigger response: %w", err)
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("trigger response missing run id")
	}

	return resp.ID, nil
}

func (c *Client) waitForCompletion(ctx context.Context, opts RunOptions, runID int, progress io.Writer) error {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		state, err := c.buildStatus(ctx, opts, runID)
		if err != nil {
			return err
		}

		if strings.EqualFold(state.Status, "completed") {
			if strings.EqualFold(state.Result, "succeeded") {
				fmt.Fprintf(progress, "%s succeeded\n", opts.Name)
				return nil
			}

			return fmt.Errorf("%s failed with result %q", opts.Name, state.Result)
		}

		fmt.Fprintf(progress, "%s status=%s result=%s\n", opts.Name, state.Status, state.Result)

		select {
		case <-ctx.Done():
			return fmt.Errorf("%s timed out after %s: %w", opts.Name, opts.Timeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *Client) buildStatus(ctx context.Context, opts RunOptions, runID int) (buildStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildStatusURL(opts, runID), nil)
	if err != nil {
		return buildStatusResponse{}, fmt.Errorf("creating status request: %w", err)
	}
	req.Header.Set("Authorization", basicAuthPAT(opts.PAT))

	respBody, err := c.do(req)
	if err != nil {
		return buildStatusResponse{}, fmt.Errorf("checking pipeline status: %w", err)
	}

	var resp buildStatusResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return buildStatusResponse{}, fmt.Errorf("decoding status response: %w", err)
	}

	return resp, nil
}

func (c *Client) cancelRun(ctx context.Context, opts RunOptions, runID int) error {
	payload := []byte(`{"status":"cancelling"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, buildStatusURL(opts, runID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", basicAuthPAT(opts.PAT))
	req.Header.Set("Content-Type", "application/json")

	_, err = c.do(req)
	return err
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

func triggerURL(opts RunOptions) string {
	return fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/pipelines/%s/runs?api-version=7.0",
		opts.Org,
		opts.Project,
		opts.PipelineID,
	)
}

func buildStatusURL(opts RunOptions, runID int) string {
	return fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_apis/build/builds/%d?api-version=7.0",
		opts.Org,
		opts.Project,
		runID,
	)
}

func buildRunURL(opts RunOptions, runID int) string {
	return fmt.Sprintf(
		"https://dev.azure.com/%s/%s/_build/results?buildId=%d&view=results",
		opts.Org,
		opts.Project,
		runID,
	)
}

func basicAuthPAT(pat string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat))
}
