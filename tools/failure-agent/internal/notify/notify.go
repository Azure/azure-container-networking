// Package notify delivers a proactive Teams alert when the failure-agent is
// confident about both the diagnosis and the proposed fix for a failed run.
//
// Delivery is an Adaptive Card POSTed to a Teams Incoming Webhook or Power
// Automate "Workflows" webhook URL (both accept the same message envelope). The
// notifier is intentionally best-effort: a missing webhook URL makes it a no-op,
// and delivery failures are surfaced to the caller to log without failing the
// run.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// adaptiveCardVersion is the schema version required for the mention entity.
const adaptiveCardVersion = "1.4"

// Mention optionally pings a specific user in the card. Both fields must be set
// for the mention to render; otherwise the card is posted without a ping.
type Mention struct {
	// UPN is the user's AAD userPrincipalName (e.g. alice@contoso.com).
	UPN string
	// Name is the display name shown in the card text.
	Name string
}

func (m Mention) valid() bool {
	return strings.TrimSpace(m.UPN) != "" && strings.TrimSpace(m.Name) != ""
}

// ShouldNotify reports whether an incident is confident enough to ping. It fires
// only when analysis succeeded, confidence meets the threshold, and the agent
// produced a proposed fix — so diagnosis-only or low-confidence runs stay quiet.
func ShouldNotify(inc model.Incident, minConfidence float64) bool {
	return inc.AnalysisStatus == model.StatusAnalyzed &&
		inc.Confidence >= minConfidence &&
		strings.TrimSpace(inc.ProposedFix) != ""
}

// RenderCard builds the Teams message envelope carrying an Adaptive Card that
// summarizes the diagnosis, proposed fix, and where the failure happened. When
// mention is valid the card pings that user.
func RenderCard(inc model.Incident, mention Mention) map[string]any {
	body := []any{
		map[string]any{
			"type":   "TextBlock",
			"size":   "Large",
			"weight": "Bolder",
			"text":   "🔎 ACN Pipeline Failure Analysis",
			"wrap":   true,
		},
	}

	if mention.valid() {
		body = append(body, map[string]any{
			"type": "TextBlock",
			"text": fmt.Sprintf("<at>%s</at>", mention.Name),
			"wrap": true,
		})
	}

	body = append(body,
		map[string]any{
			"type": "FactSet",
			"facts": []any{
				fact("Category", string(inc.Category)),
				fact("Confidence", fmt.Sprintf("%s (%.2f)", inc.ConfidenceBand, inc.Confidence)),
				fact("Pipeline", dash(inc.PipelineName)),
				fact("Stage / Job", dash(strings.Trim(inc.Stage+" / "+inc.Job, " /"))),
				fact("Cluster", dash(inc.ClusterName)),
			},
		},
		heading("Likely root cause"),
		paragraph(dash(inc.RootCauseSummary)),
		heading("Proposed fix"),
		paragraph(dash(inc.ProposedFix)),
	)

	card := map[string]any{
		"type":    "AdaptiveCard",
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"version": adaptiveCardVersion,
		"body":    body,
	}

	if link := prURL(inc); link != "" {
		card["actions"] = []any{
			map[string]any{
				"type":  "Action.OpenUrl",
				"title": fmt.Sprintf("Open PR #%s", inc.PullRequestNumber),
				"url":   link,
			},
		}
	}

	if mention.valid() {
		card["msteams"] = map[string]any{
			"entities": []any{
				map[string]any{
					"type": "mention",
					"text": fmt.Sprintf("<at>%s</at>", mention.Name),
					"mentioned": map[string]any{
						"id":   mention.UPN,
						"name": mention.Name,
					},
				},
			},
		}
	}

	return map[string]any{
		"type": "message",
		"attachments": []any{
			map[string]any{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content":     card,
			},
		},
	}
}

// Notifier delivers a rendered card payload. main injects the HTTP-backed
// implementation; tests inject a fake.
type Notifier interface {
	Send(ctx context.Context, payload map[string]any) error
}

// WebhookNotifier POSTs card payloads to a Teams webhook URL.
type WebhookNotifier struct {
	URL    string
	Client *http.Client
}

// Send marshals payload and POSTs it, returning an error on transport failure or
// a non-2xx response.
func (w WebhookNotifier) Send(ctx context.Context, payload map[string]any) error {
	client := w.Client
	if client == nil {
		client = http.DefaultClient
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling teams payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("building teams request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("posting to teams webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("teams webhook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

func fact(title, value string) map[string]any {
	return map[string]any{"title": title, "value": value}
}

func heading(text string) map[string]any {
	return map[string]any{
		"type":      "TextBlock",
		"weight":    "Bolder",
		"text":      text,
		"separator": true,
		"wrap":      true,
	}
}

func paragraph(text string) map[string]any {
	return map[string]any{"type": "TextBlock", "text": text, "wrap": true}
}

func prURL(inc model.Incident) string {
	if inc.PullRequestNumber == "" || inc.Repository == "" {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/pull/%s", inc.Repository, inc.PullRequestNumber)
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
