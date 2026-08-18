package collect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
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
	if len(ev.ErrorSnippets) == 0 {
		t.Fatal("expected line-numbered error snippets")
	}
	first := ev.ErrorSnippets[0]
	if first.File != "pods.log" {
		t.Errorf("snippet file: got %q, want pods.log", first.File)
	}
	if first.Line <= 0 {
		t.Errorf("snippet line: got %d", first.Line)
	}
	if !strings.Contains(first.Snippet, "|") {
		t.Errorf("snippet missing line-number context: %q", first.Snippet)
	}
	if !strings.Contains(ev.Excerpts["pods.log"], "match line") {
		t.Errorf("expected excerpt to include match line header: %q", ev.Excerpts["pods.log"])
	}
}

func TestParseEvidenceSurfacesNodeHealthWithoutErrors(t *testing.T) {
	dir := t.TempDir()
	// node-status.txt has no error keywords but is essential node evidence.
	writeFile(t, dir, "node-status.txt", "NAME                              STATUS     ROLES   AGE\naks-nodepool1-vmss000000          NotReady   agent   42m\n")
	writeFile(t, dir, "unrelated.txt", "everything healthy\nready\n")

	ev, err := ParseEvidence(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	excerpt, ok := ev.Excerpts["node-status.txt"]
	if !ok {
		t.Fatalf("expected node-status.txt to be surfaced as an excerpt, got %v", ev.Excerpts)
	}
	if !strings.Contains(excerpt, "NotReady") {
		t.Errorf("expected node status content in excerpt: %q", excerpt)
	}
	if _, ok := ev.Excerpts["unrelated.txt"]; ok {
		t.Error("did not expect excerpt for a non-node file with no errors")
	}
}

func TestIsNodeEvidenceFile(t *testing.T) {
	yes := []string{"node-status.txt", "node-network-configs.txt", "logs/node-conditions.txt", "nodes", "nodes.txt"}
	for _, n := range yes {
		if !isNodeEvidenceFile(n) {
			t.Errorf("expected %q to be node evidence", n)
		}
	}
	no := []string{"pods.txt", "azure-cns.log", "kube-system/coredns-node-manager-logs.txt"}
	for _, n := range no {
		if isNodeEvidenceFile(n) {
			t.Errorf("did not expect %q to be node evidence", n)
		}
	}
}

func TestParseEvidenceSurfacesDatapathStateWithoutErrors(t *testing.T) {
	dir := t.TempDir()
	// cnsCache.txt is pure IP-allocation state with no error keywords, but is
	// decisive datapath evidence and must be surfaced as an excerpt.
	writeFile(t, dir, "cnsCache.txt", "NCID: abc123\nTotal IPs: 256\nAllocated: 256\nAvailable: 0\n")
	writeFile(t, dir, "unrelated.txt", "everything healthy\nready\n")

	ev, err := ParseEvidence(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	excerpt, ok := ev.Excerpts["cnsCache.txt"]
	if !ok {
		t.Fatalf("expected cnsCache.txt to be surfaced as an excerpt, got %v", ev.Excerpts)
	}
	if !strings.Contains(excerpt, "Available: 0") {
		t.Errorf("expected IP-allocation state in excerpt: %q", excerpt)
	}
	if _, ok := ev.Excerpts["unrelated.txt"]; ok {
		t.Error("did not expect excerpt for a non-datapath file with no errors")
	}
}

func TestIsDatapathEvidenceFile(t *testing.T) {
	yes := []string{
		"CNS-output/azure-cns.json", "cnsCache.txt", "CNS-output/azure-endpoints.json",
		"HNS-output/hns-endpoint.json", "HNS-output/hns-network.json",
		"log-output/azure-cns.log", "log-output/azure-vnet.log",
		"full-windows-logs/extracted/endpoint.txt", "full-windows-logs/extracted/routes.txt",
		"full-windows-logs/extracted/ports.txt", "full-windows-logs/extracted/vfpOutput.txt",
		"full-windows-logs/extracted/ip.txt",
	}
	for _, n := range yes {
		if !isDatapathEvidenceFile(n) {
			t.Errorf("expected %q to be datapath evidence", n)
		}
	}
	no := []string{
		"node-status.txt", "full-windows-logs/extracted/arp.txt",
		"full-windows-logs/extracted/firewall.txt", "full-windows-logs/extracted/reservedports.txt",
		"log-output/azure-vnet-telemetry.log", "full-windows-logs/extracted/adapters/Ethernet 3_int.txt",
	}
	for _, n := range no {
		if isDatapathEvidenceFile(n) {
			t.Errorf("did not expect %q to be datapath evidence", n)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// TestParseEvidenceDrawsErrorLinesAcrossFiles pins the round-robin draw. The walk
// is lexical, so a node's containerd journal (1000 lines) is read before the CNS
// log and before the captured task logs; taken in walk order it fills the whole
// budget and the decisive lines never appear.
func TestParseEvidenceDrawsErrorLinesAcrossFiles(t *testing.T) {
	dir := t.TempDir()

	var noisy strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&noisy, "containerd error %d: failed to handle event\n", i)
	}
	if err := os.MkdirAll(filepath.Join(dir, "aks-node_logs", "containerd-output"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "aks-node_logs", "log-output"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "aks-node_logs", "containerd-output"), "containerd.log", noisy.String())
	writeFile(t, filepath.Join(dir, "aks-node_logs", "log-output"), "azure-cns.log",
		"[releaseIPConfigs] Failed to release IP 10.244.1.9 for pod default/pod-z error: not found\n")

	ev, err := ParseEvidence(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(strings.Join(ev.TopErrorLines, "\n"), "Failed to release IP") {
		t.Errorf("CNS release failure was crowded out of the error-line budget: %v", ev.TopErrorLines)
	}
}

// TestParseEvidenceKeepsAssertionExcerptPastCap pins that the captured task log,
// which carries the only copy of the test's assertion text, is not dropped when
// the lexically-earlier per-node files have already filled the excerpt cap.
func TestParseEvidenceKeepsAssertionExcerptPastCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < maxExcerptFiles+5; i++ {
		writeFile(t, dir, fmt.Sprintf("aaa-filler-%02d.log", i), "error: filler\n")
	}
	if err := os.MkdirAll(filepath.Join(dir, "e2e-task-logs"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "e2e-task-logs"), "Validate_Node_Restart_1234.txt",
		"State file validation failed: Remaining, potentially leaked, IP(s) on state file - map[10.244.1.9:pod-z]\n")

	ev, err := ParseEvidence(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := ev.Excerpts["e2e-task-logs/Validate_Node_Restart_1234.txt"]; !ok {
		t.Error("assertion task log was dropped by the excerpt cap")
	}
}

func TestLoadChangeContextExtractsFilesAndBody(t *testing.T) {
	diff := "diff --git a/cns/restserver/ipam.go b/cns/restserver/ipam.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/cns/restserver/ipam.go\n" +
		"+++ b/cns/restserver/ipam.go\n" +
		"@@ -1047,7 +1047,7 @@\n" +
		"-		if ipState.GetState() != types.Available {\n" +
		"+		if ipState.GetState() != types.Available && !reuseExhausted {\n" +
		"diff --git a/cns/restserver/ipam_test.go b/cns/restserver/ipam_test.go\n" +
		"--- a/cns/restserver/ipam_test.go\n" +
		"+++ b/cns/restserver/ipam_test.go\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"

	path := filepath.Join(t.TempDir(), "change.diff")
	if err := os.WriteFile(path, []byte(diff), 0o600); err != nil {
		t.Fatalf("writing diff: %v", err)
	}

	var rc model.RunContext
	if err := LoadChangeContext(&rc, path); err != nil {
		t.Fatalf("LoadChangeContext: %v", err)
	}

	want := []string{"cns/restserver/ipam.go", "cns/restserver/ipam_test.go"}
	if strings.Join(rc.ChangedFiles, ",") != strings.Join(want, ",") {
		t.Errorf("changed files = %v, want %v", rc.ChangedFiles, want)
	}
	if !strings.Contains(rc.Diff, "types.Available") {
		t.Error("diff body was not retained")
	}
}

func TestLoadChangeContextEmptyDiffIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "change.diff")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("writing diff: %v", err)
	}

	var rc model.RunContext
	if err := LoadChangeContext(&rc, path); err != nil {
		t.Fatalf("LoadChangeContext on empty diff: %v", err)
	}
	if len(rc.ChangedFiles) != 0 || rc.Diff != "" {
		t.Errorf("expected no change context, got files=%v diff=%q", rc.ChangedFiles, rc.Diff)
	}
}

func TestLoadChangeContextTruncatesLargeDiff(t *testing.T) {
	body := "+++ b/big.go\n" + strings.Repeat("+line of change\n", 20000)
	path := filepath.Join(t.TempDir(), "change.diff")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing diff: %v", err)
	}

	var rc model.RunContext
	if err := LoadChangeContext(&rc, path); err != nil {
		t.Fatalf("LoadChangeContext: %v", err)
	}
	if len(rc.Diff) > maxDiffChars+64 {
		t.Errorf("diff not truncated: %d bytes", len(rc.Diff))
	}
	if !strings.Contains(rc.Diff, "diff truncated") {
		t.Error("expected a truncation marker")
	}
}
