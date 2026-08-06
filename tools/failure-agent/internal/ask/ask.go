// Package ask implements the failure-agent "ask mode": it answers an analyst's
// follow-up question about a specific failed build, grounded in that build's
// prior FAA analysis and its collected evidence bundle. Unlike the classify
// path it does not re-triage, re-score, or emit an incident; it returns a
// free-form Markdown answer.
package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/classify"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/report"
)

// AnswerFile is the Markdown artifact ask mode writes to the output directory.
const AnswerFile = "answer.md"

// PriorAnalysis is the target build's earlier FAA verdict, loaded from its
// failureAnalysis_* artifact. Either field may be empty when the artifact is
// missing or partial; the answerer still grounds on whatever is present.
type PriorAnalysis struct {
	// Report is the contents of the prior report.md, if present.
	Report string
	// Incident is the parsed prior incident.json, or nil when absent/unparseable.
	Incident *model.Incident
}

// LoadPriorAnalysis reads report.md and incident.json from dir. An empty dir
// yields an empty PriorAnalysis with no error. A non-empty dir that does not
// exist is an error; individual missing files degrade gracefully so ask mode
// can still answer from evidence alone.
func LoadPriorAnalysis(dir string) (PriorAnalysis, error) {
	if dir == "" {
		return PriorAnalysis{}, nil
	}
	if _, err := os.Stat(dir); err != nil {
		return PriorAnalysis{}, fmt.Errorf("prior-analysis dir: %w", err)
	}

	var pa PriorAnalysis
	if b, err := os.ReadFile(filepath.Join(dir, report.MarkdownFile)); err == nil {
		pa.Report = string(b)
	}
	if b, err := os.ReadFile(filepath.Join(dir, report.JSONFile)); err == nil {
		var inc model.Incident
		if jsonErr := json.Unmarshal(b, &inc); jsonErr == nil {
			pa.Incident = &inc
		}
	}
	return pa, nil
}

// Answerer produces a free-form Markdown answer to an analyst question, grounded
// in the run's evidence and prior FAA analysis. It calls the ChatCompleter
// without a JSON schema so the model returns prose, never a classification.
type Answerer struct {
	client classify.ChatCompleter
}

// NewAnswerer returns an Answerer backed by client.
func NewAnswerer(client classify.ChatCompleter) *Answerer {
	return &Answerer{client: client}
}

// Answer asks the model the question grounded in prior analysis and evidence and
// returns the raw Markdown response. An empty question or empty model response
// is an error so the caller can fail loudly rather than post an empty reply.
func (a *Answerer) Answer(ctx context.Context, question string, prior PriorAnalysis, ev model.Evidence) (string, error) {
	if strings.TrimSpace(question) == "" {
		return "", errors.New("question is required")
	}
	raw, err := a.client.Complete(ctx, systemPrompt(), userPrompt(question, prior, ev), nil)
	if err != nil {
		return "", fmt.Errorf("llm completion: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("llm returned an empty answer")
	}
	return raw, nil
}
