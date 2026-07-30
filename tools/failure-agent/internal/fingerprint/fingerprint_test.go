package fingerprint

import (
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

func TestNormalizeStripsNoise(t *testing.T) {
	in := "2024-01-02T03:04:05Z Error dialing 10.0.0.1 for pod cns-abcd1234ef99 id 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d count 42"
	got := Normalize(in)

	for _, token := range []string{"<ts>", "<ip>", "<guid>", "<n>"} {
		if !strings.Contains(got, token) {
			t.Errorf("expected %q in normalized output, got %q", token, got)
		}
	}
	for _, raw := range []string{"10.0.0.1", "1a2b3c4d-5e6f", "2024-01-02"} {
		if strings.Contains(got, raw) {
			t.Errorf("expected %q to be scrubbed, got %q", raw, got)
		}
	}
}

func TestComputeStableAcrossNoise(t *testing.T) {
	rc := model.RunContext{PipelineName: "ACN", StageName: "Cilium", JobName: "e2e", ClusterType: "overlay", OS: "linux", CNI: "cilium"}

	a := model.Evidence{TopErrorLines: []string{
		"2024-01-02T03:04:05Z dial tcp 10.0.0.1:443: connection refused",
		"pod coredns-7f9c8b6d4-xz12k crashed at commit deadbeefcafe1",
	}}
	b := model.Evidence{TopErrorLines: []string{
		"2025-09-09T09:09:09Z dial tcp 172.16.4.9:443: connection refused",
		"pod coredns-66b6c48dd5-9p2mf crashed at commit feedface1234",
	}}

	fa := Compute(rc, a)
	fb := Compute(rc, b)

	if fa.Hash == "" {
		t.Fatal("expected non-empty fingerprint hash")
	}
	if fa.Hash != fb.Hash {
		t.Errorf("expected identical fingerprints across run-specific noise:\n a=%s (%s)\n b=%s (%s)", fa.Hash, fa.NormalizedSignal, fb.Hash, fb.NormalizedSignal)
	}
}

func TestComputeDiffersOnDifferentFailure(t *testing.T) {
	rc := model.RunContext{PipelineName: "ACN", StageName: "Cilium"}
	a := Compute(rc, model.Evidence{TopErrorLines: []string{"ImagePullBackOff for azure-cns"}})
	b := Compute(rc, model.Evidence{TopErrorLines: []string{"context deadline exceeded waiting for pods"}})

	if a.Hash == b.Hash {
		t.Errorf("expected different fingerprints for different failures, both = %s", a.Hash)
	}
}
