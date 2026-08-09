package ask

import (
	"context"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/corpus"
)

func TestAnswerFromHistoryBuildsGroundedPrompt(t *testing.T) {
	fc := &fakeCompleter{response: "Yes, build 123 saw the same CNS restart; the fix was to bump the CNI."}
	records := []corpus.Record{
		{Path: "123/report.md", Markdown: "# Failure Analysis\nCNS restarted after node reboot."},
		{Path: "456/report.md", Markdown: "# Failure Analysis\nunrelated image pull error."},
	}

	got, err := NewAnswerer(fc).AnswerFromHistory(context.Background(), "has CNS restarted before?", records)
	if err != nil {
		t.Fatalf("AnswerFromHistory: %v", err)
	}
	if got != fc.response {
		t.Fatalf("answer = %q, want %q", got, fc.response)
	}
	if fc.gotSchema != nil {
		t.Error("history ask must pass no JSON schema")
	}
	if !strings.Contains(fc.gotUser, "has CNS restarted before?") {
		t.Error("question missing from prompt")
	}
	if !strings.Contains(fc.gotUser, "123/report.md") || !strings.Contains(fc.gotUser, "CNS restarted after node reboot") {
		t.Error("historical report content/citation missing from prompt")
	}
	if !strings.Contains(strings.ToLower(fc.gotSystem), "do not re-triage") {
		t.Error("system prompt should forbid re-triage")
	}
}

func TestAnswerFromHistoryEmptyQuestion(t *testing.T) {
	fc := &fakeCompleter{response: "x"}
	if _, err := NewAnswerer(fc).AnswerFromHistory(context.Background(), "  ", nil); err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestAnswerFromHistoryNoRecords(t *testing.T) {
	fc := &fakeCompleter{response: "No relevant precedent found in the corpus."}
	if _, err := NewAnswerer(fc).AnswerFromHistory(context.Background(), "any precedent?", nil); err != nil {
		t.Fatalf("AnswerFromHistory with no records: %v", err)
	}
	if !strings.Contains(fc.gotUser, "no historical reports available") {
		t.Error("empty-corpus prompt should state no reports available")
	}
}

func TestAnswerFromHistoryEmptyResponseIsError(t *testing.T) {
	fc := &fakeCompleter{response: "   "}
	if _, err := NewAnswerer(fc).AnswerFromHistory(context.Background(), "q", nil); err == nil {
		t.Fatal("expected error for empty model response")
	}
}
