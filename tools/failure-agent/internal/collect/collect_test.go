package collect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromEnvMapsFields(t *testing.T) {
	env := map[string]string{
		"BUILD_DEFINITIONNAME":                 "Azure Container Networking PR",
		"BUILD_BUILDID":                        "12345",
		"BUILD_REPOSITORY_NAME":                "Azure/azure-container-networking",
		"SYSTEM_STAGEDISPLAYNAME":              "Cilium Overlay E2E",
		"SYSTEM_JOBDISPLAYNAME":                "e2e",
		"BUILD_REASON":                         "PullRequest",
		"SYSTEM_PULLREQUEST_PULLREQUESTNUMBER": "987",
		"SYSTEM_PULLREQUEST_TARGETBRANCH":      "refs/heads/master",
		"SYSTEM_PULLREQUEST_SOURCECOMMITID":    "abcdef0",
	}
	rc := FromEnv(func(k string) string { return env[k] })

	if rc.PipelineName != "Azure Container Networking PR" {
		t.Errorf("pipeline name: got %q", rc.PipelineName)
	}
	if rc.StageName != "Cilium Overlay E2E" {
		t.Errorf("stage name: got %q", rc.StageName)
	}
	if !rc.IsPR {
		t.Error("expected IsPR true")
	}
	if rc.PullRequestNumber != "987" {
		t.Errorf("pr number: got %q", rc.PullRequestNumber)
	}
}

func TestParseEvidenceExtractsErrorsAndDedups(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pods.log", "all good\nImagePullBackOff azure-cns\nError: something failed\nImagePullBackOff azure-cns\n")
	writeFile(t, dir, "clean.txt", "everything healthy\nready\n")

	ev, err := ParseEvidence(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ev.Files) != 2 {
		t.Errorf("expected 2 files listed, got %d: %v", len(ev.Files), ev.Files)
	}
	if len(ev.TopErrorLines) != 2 {
		t.Errorf("expected 2 deduped error lines, got %d: %v", len(ev.TopErrorLines), ev.TopErrorLines)
	}
	if _, ok := ev.Excerpts["pods.log"]; !ok {
		t.Errorf("expected excerpt for pods.log, got %v", ev.Excerpts)
	}
	if _, ok := ev.Excerpts["clean.txt"]; ok {
		t.Error("did not expect excerpt for a file with no errors")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}
