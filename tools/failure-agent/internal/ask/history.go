package ask

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/corpus"
)

// AnswerFromHistory answers a build-independent question grounded in a set of
// historical FAA reports pulled from the corpus. Like build-scoped ask it calls
// the ChatCompleter without a JSON schema so the model returns prose, never a
// classification. An empty question or empty model response is an error so the
// caller fails loudly rather than posting an empty reply.
func (a *Answerer) AnswerFromHistory(ctx context.Context, question string, records []corpus.Record) (string, error) {
	if strings.TrimSpace(question) == "" {
		return "", errors.New("question is required")
	}
	raw, err := a.client.Complete(ctx, historySystemPrompt(), historyUserPrompt(question, records), nil)
	if err != nil {
		return "", fmt.Errorf("llm completion: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("llm returned an empty answer")
	}
	return raw, nil
}

// historySystemPrompt frames history mode as grounded question-answering across
// past failures, not re-triage of any single run.
func historySystemPrompt() string {
	return "You are an expert Azure Container Networking (ACN) CI failure analyst " +
		"answering an engineer's question by drawing on a corpus of prior automated " +
		"failure-analysis reports from past pipeline runs. " +
		"You are given the engineer's question and a set of past FAA reports (each with a source path). " +
		"Answer the question directly and concisely in Markdown, grounded strictly in the provided reports. " +
		"Cite the specific report source paths you rely on. Look across reports for recurring patterns, " +
		"prior occurrences of the same failure, and previously proposed fixes. " +
		"When the provided reports do not contain a relevant precedent, say so explicitly and state what " +
		"would help — do not speculate or fabricate. " +
		"This is a grounded question-answer task over historical reports: do NOT re-triage any failure, and " +
		"do NOT output a new classification, category, confidence score, or JSON — respond only with the prose answer."
}

// historyUserPrompt assembles the question and the selected historical reports.
func historyUserPrompt(question string, records []corpus.Record) string {
	var b strings.Builder

	b.WriteString("## Question\n")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n\n")

	b.WriteString("## Historical FAA reports\n")
	if len(records) == 0 {
		b.WriteString("(no historical reports available in the corpus)\n")
		return b.String()
	}
	for _, r := range records {
		fmt.Fprintf(&b, "### Report: %s\n", r.Path)
		b.WriteString(strings.TrimSpace(r.Markdown))
		b.WriteString("\n\n")
	}
	return b.String()
}
