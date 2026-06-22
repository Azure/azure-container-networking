package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const SourceScheduledRelease = "scheduled-release"

type NotificationRequest struct {
	Source    string `json:"source"`
	RunID     string `json:"runId"`
	TeamID    string `json:"teamId"`
	ChannelID string `json:"channelId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Summary   string `json:"summary"`
	Stage     string `json:"stage,omitempty"`
}

type ReplyRequest struct {
	Source   string `json:"source"`
	RunID    string `json:"runId"`
	Text     string `json:"text"`
	Tag      string `json:"tag,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type execRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type Client struct {
	baseURL  string
	audience string
	doer     httpDoer
	run      execRunner
}

func NewClientFromEnv() *Client {
	return &Client{
		baseURL:  strings.TrimRight(os.Getenv("NOTIFIER_URL"), "/"),
		audience: os.Getenv("NOTIFIER_AUDIENCE"),
		doer:     http.DefaultClient,
		run:      runCommand,
	}
}

func DefaultTeamID() string {
	return os.Getenv("TEAMS_TEAM_ID")
}

func DefaultRunID() string {
	runID := strings.TrimSpace(os.Getenv("GITHUB_RUN_ID"))
	if runID == "" {
		return ""
	}

	return "scheduled-release-" + runID
}

func (c *Client) Notify(ctx context.Context, req NotificationRequest) ([]byte, error) {
	return c.post(ctx, "/api/notifications", req)
}

func (c *Client) Reply(ctx context.Context, req ReplyRequest) ([]byte, error) {
	return c.post(ctx, "/api/notifications/reply", req)
}

func (c *Client) post(ctx context.Context, path string, payload any) ([]byte, error) {
	if strings.TrimSpace(c.baseURL) == "" {
		return nil, fmt.Errorf("NOTIFIER_URL is not set")
	}
	if strings.TrimSpace(c.audience) == "" {
		return nil, fmt.Errorf("NOTIFIER_AUDIENCE is not set")
	}

	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.doer.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return respBody, fmt.Errorf("notifier returned status %d", resp.StatusCode)
	}

	return respBody, nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "az", "account", "get-access-token", "--resource", c.audience, "--query", "accessToken", "-o", "tsv")
	if err != nil {
		return "", fmt.Errorf("acquiring notifier token: %w", err)
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("acquiring notifier token: empty token")
	}

	return token, nil
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return out, nil
}
