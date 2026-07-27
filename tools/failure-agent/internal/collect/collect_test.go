package collect

import (
	"os"
	"path/filepath"
	"strings"
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
