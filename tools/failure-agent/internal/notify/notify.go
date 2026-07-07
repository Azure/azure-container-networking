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
// Use this when your org allows incoming webhooks / Power Automate triggers.
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

// GraphConfig holds the AAD app registration credentials and target chat/channel
// for posting via Microsoft Graph API. Use this when DLP policy blocks webhook triggers.
type GraphConfig struct {
	TenantID     string // AAD tenant ID
	ClientID     string // App registration client ID
	ClientSecret string // App registration client secret
	// Exactly one of ChatID or (TeamID + ChannelID) must be set.
	ChatID    string // For 1:1 or group chat messages
	TeamID    string // For channel messages
	ChannelID string // For channel messages
}

// GraphNotifier posts Adaptive Card messages via the Microsoft Graph API using
// client_credentials OAuth2 flow. It does not depend on Power Automate or
// incoming webhooks, so it works even when those are blocked by DLP policy.
type GraphNotifier struct {
	Config GraphConfig
	Client *http.Client
}

// Send acquires an access token and posts the card payload as a chatMessage.
// The payload is re-wrapped into the Graph chatMessage format (the caller still
// passes the same RenderCard output).
func (g GraphNotifier) Send(ctx context.Context, payload map[string]any) error {
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}

	token, err := g.acquireToken(ctx, client)
	if err != nil {
		return fmt.Errorf("acquiring graph token: %w", err)
	}

	graphMsg := toGraphMessage(payload)
	data, err := json.Marshal(graphMsg)
	if err != nil {
		return fmt.Errorf("marshaling graph message: %w", err)
	}

	url := g.messagesURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("building graph request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("posting to graph: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("graph api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}

// acquireToken performs the OAuth2 client_credentials flow against the Microsoft
// identity platform to get a Graph API access token.
func (g GraphNotifier) acquireToken(ctx context.Context, client *http.Client) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", g.Config.TenantID)
	body := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://graph.microsoft.com/.default",
		g.Config.ClientID, g.Config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(snippet))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}
	return tokenResp.AccessToken, nil
}

func (g GraphNotifier) messagesURL() string {
	if g.Config.ChatID != "" {
		return fmt.Sprintf("https://graph.microsoft.com/v1.0/chats/%s/messages", g.Config.ChatID)
	}
	return fmt.Sprintf("https://graph.microsoft.com/v1.0/teams/%s/channels/%s/messages", g.Config.TeamID, g.Config.ChannelID)
}

// toGraphMessage converts the webhook-format payload (from RenderCard) into the
// Microsoft Graph chatMessage format with an Adaptive Card attachment.
func toGraphMessage(payload map[string]any) map[string]any {
	// Extract the adaptive card content from the webhook envelope.
	var cardContent any
	if attachments, ok := payload["attachments"].([]any); ok && len(attachments) > 0 {
		if att, ok := attachments[0].(map[string]any); ok {
			cardContent = att["content"]
		}
	}
	if cardContent == nil {
		cardContent = map[string]any{}
	}

	// Graph requires the card JSON as a string, not an object.
	cardJSON, _ := json.Marshal(cardContent)

	const attachmentID = "acn-failure-card"
	return map[string]any{
		"body": map[string]any{
			"contentType": "html",
			"content":     fmt.Sprintf(`<attachment id="%s"></attachment>`, attachmentID),
		},
		"attachments": []any{
			map[string]any{
				"id":          attachmentID,
				"contentType": "application/vnd.microsoft.card.adaptive",
				"contentUrl":  nil,
				"content":     string(cardJSON),
			},
		},
	}
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
