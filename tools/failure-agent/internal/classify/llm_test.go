package classify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

type fakeCompleter struct {
	response string
	err      error

	gotSystem string
	gotUser   string
	gotSchema *Schema
}

func (f *fakeCompleter) Complete(_ context.Context, system, user string, schema *Schema) (string, error) {
	f.gotSystem = system
	f.gotUser = user
	f.gotSchema = schema
	return f.response, f.err
}

func TestLLMClassifierPromptCoversNodeHealth(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "pipeline_infra_config",
		"confidence": 0.6,
		"rootCauseSummary": "node rebooted",
		"topEvidence": ["NotReady"],
		"recommendedOwner": "acn-infra",
		"proposedFix": "rerun once nodepool healthy",
		"nodeAssessment": "node aks-nodepool1-vmss000000 went NotReady after reboot"
	}`}

	got, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !strings.Contains(fc.gotSystem, "node and nodepool health") {
		t.Error("expected system prompt to direct node/nodepool investigation")
	}
	if got.NodeAssessment == "" {
		t.Error("expected node assessment to be propagated from the model response")
	}
}

func TestWriteExcerptsPrioritizesNodeEvidence(t *testing.T) {
	filler := strings.Repeat("x", maxExcerptChars)
	excerpts := map[string]string{
		"aks-nodepool1-vmss000000_logs/containerd-output/containerd.log": filler,
		"live/cilium-logs":     filler,
		"live/cns-logs":        filler,
		"live/nodes":           "NODE STATUS: aks-nodepool1-vmss000000 NotReady",
		"node-status.txt":      "node rebooted at 07:02",
		"node-network-configs": filler,
	}
	var b strings.Builder
	writeExcerpts(&b, excerpts)
	out := b.String()

	if !strings.Contains(out, "live/nodes") || !strings.Contains(out, "NotReady") {
		t.Error("expected node evidence to survive the excerpt budget")
	}
	if !strings.Contains(out, "node-status.txt") {
		t.Error("expected node-status.txt excerpt to be prioritized")
	}
}

func TestWriteExcerptsPrioritizesDatapathEvidence(t *testing.T) {
	filler := strings.Repeat("x", maxExcerptChars)
	excerpts := map[string]string{
		"aaa_logs/a.log": filler,
		"aab_logs/b.log": filler,
		"aac_logs/c.log": filler,
		"aad_logs/d.log": filler,
		"aae_logs/e.log": filler,
		"live/nnc":       "NNC nodepool1: requested 256 allocated 0",
		"aksnpwin000000_logs/CNS-output/azure-cns.json": "IPAMPoolMonitor cached 0 free 0",
	}
	var b strings.Builder
	writeExcerpts(&b, excerpts)
	out := b.String()

	if !strings.Contains(out, "live/nnc") || !strings.Contains(out, "requested 256 allocated 0") {
		t.Error("expected NNC allocation state to survive the excerpt budget")
	}
	if !strings.Contains(out, "CNS-output/azure-cns.json") {
		t.Error("expected bundle datapath state to be prioritized into the budget")
	}
}

func TestLLMClassifierValidResponse(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "pr_regression",
		"confidence": 0.91,
		"rootCauseSummary": "the change under test removed a required field",
		"topEvidence": ["panic: nil pointer", "added in this PR"],
		"recommendedOwner": "acn-cni",
		"proposedFix": "Restore the required field in the struct and add a nil check before accessing it."
	}`}

	got, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify returned error: %v", err)
	}

	if got.Category != model.CategoryPRRegression {
		t.Errorf("category: got %s, want %s", got.Category, model.CategoryPRRegression)
	}
	if got.Confidence != 0.91 {
		t.Errorf("confidence: got %v, want 0.91", got.Confidence)
	}
	if got.Source != "llm" {
		t.Errorf("source: got %q, want llm", got.Source)
	}
	if got.RootCauseSummary == "" {
		t.Error("expected non-empty root cause summary")
	}
	if fc.gotSchema == nil || fc.gotSchema.Name == "" {
		t.Error("expected a schema to be passed to the completer")
	}
}

func TestLLMClassifierRejectsBadResponses(t *testing.T) {
	tests := map[string]string{
		"invalid category": `{"category":"definitely_not_real","confidence":0.5,"rootCauseSummary":"x"}`,
		"confidence high":  `{"category":"known_flake","confidence":1.5,"rootCauseSummary":"x"}`,
		"confidence low":   `{"category":"known_flake","confidence":-0.1,"rootCauseSummary":"x"}`,
		"empty summary":    `{"category":"known_flake","confidence":0.5,"rootCauseSummary":"   "}`,
		"not json":         `not json at all`,
	}

	for name, resp := range tests {
		t.Run(name, func(t *testing.T) {
			fc := &fakeCompleter{response: resp}
			if _, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{}); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestLLMClassifierPropagatesCompleterError(t *testing.T) {
	fc := &fakeCompleter{err: errors.New("boom")}
	if _, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{}); err == nil {
		t.Fatal("expected error when completer fails")
	}
}

func TestSystemPromptEncodesInvestigationPolicy(t *testing.T) {
	sp := systemPrompt()
	for _, want := range []string{
		"evidence-first, verdict-last",
		"most severe anomaly",
		"Symptom vs cause",
		"Falsification via cross-dimension correlation",
		"FINAL VERDICT",
		"finalVerdict",
		"2000-character",
		"DATAPATH / IP-PLANE EVIDENCE",
		"live/nnc",
		"failingUnit",
		"knownUnknowns",
		"~1h TTL",
		"cross-commit/cross-stage",
		"ANTI-PATTERNS",
	} {
		if !strings.Contains(sp, want) {
			t.Errorf("system prompt missing investigation-policy element %q", want)
		}
	}
}

func TestClassificationSchemaIncludesContractFields(t *testing.T) {
	def := string(classificationSchema().Definition)
	for _, want := range []string{
		"finalVerdict", "topAnomaly", "failingUnit", "causalChain", "symptomVsCause",
		"falsification", "evidenceGaps", "knownUnknowns",
	} {
		if !strings.Contains(def, want) {
			t.Errorf("classification schema missing contract field %q", want)
		}
	}
}

// TestLLMClassifierParsesFullContract feeds a golden-shaped response and asserts
// the structured triage contract is parsed onto the Classification.
func TestLLMClassifierParsesFullContract(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "pipeline_infra_config",
		"confidence": 0.72,
		"rootCauseSummary": "Defender init container install-packages.sh exited 1 under chroot and crashlooped the DaemonSet.",
		"finalVerdict": "Root cause - confirmed from the pod's embedded init script\n\nThe describe output embeds the installer exit path and shows PackageInstallFailed before exit 1.\n\nPlain text:\n1044 emit_k8s_event \"Warning\" \"PackageInstallFailed\" \"$_msg\"\n1045 echo \"ERROR: install-packages.sh failed with exit code $install_rc\"\n1048 exit $install_rc\n\nVerdict: AzSecPack/node-image infra, not an ACN/CNS regression. Capture kubectl logs <pod> -c init-package-installer --previous before event TTL.",
		"topAnomaly": "kube-system Defender init container CrashLoopBackOff x289 over 75m",
		"failingUnit": "install-packages.sh in the azsecpack init-package-installer container",
		"topEvidence": ["install-packages.sh failed with exit code", "Init:CrashLoopBackOff x289"],
		"causalChain": [
			{"step": "Nodes rebooted", "timestamp": "2026-07-27T08:59:00Z", "citation": "node-conditions.txt"},
			{"step": "install-packages.sh exited 1", "timestamp": "", "citation": "bad-pod-describe.txt line 1048"},
			{"step": "init container CrashLoopBackOff blocked SynchronizedBeforeSuite", "timestamp": "", "citation": "pods.txt"}
		],
		"symptomVsCause": [
			{"signal": "CNS connection refused", "classification": "symptom", "justification": "CNS restarted during the same node disruption"},
			{"signal": "install-packages.sh exit 1", "classification": "cause", "justification": "primary init failure quoted from the script"}
		],
		"falsification": {
			"hypothesis": "CNS IPAM code regression",
			"ifTrueExpect": "failure localized to CNS on the changed datapath",
			"ifFalseExpect": "identical signature across nodes/stages independent of the change",
			"correlationResult": "identical init exit 1 across 2 nodepools and 2 stages on the same commit and image tag",
			"outcome": "refuted"
		},
		"evidenceGaps": [
			{"missing": "literal dpkg PackageInstallFailed message", "whereItLives": "expired k8s event and in-pod install log", "whyMissing": "events ~1h TTL; capture was 75m in", "howToCapture": "kubectl logs <pod> -c init-package-installer --previous"}
		],
		"knownUnknowns": ["exact failing .deb postinst step not captured this run"],
		"recommendedOwner": "aks-node-image",
		"proposedFix": "Route to node-image/AzSecPack; capture the installer log next run.",
		"nodeAssessment": "Both nodes rebooted ~08:59 (durable Kubelet transition time); CNS/Defender restarts are side effects."
	}`}

	got, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Category != model.CategoryPipelineInfraConfig {
		t.Errorf("category: got %s, want pipeline_infra_config", got.Category)
	}
	if !strings.Contains(got.FailingUnit, "install-packages.sh") {
		t.Errorf("failingUnit: got %q", got.FailingUnit)
	}
	if !strings.Contains(got.FinalVerdict, "Root cause") || !strings.Contains(got.FinalVerdict, "PackageInstallFailed") {
		t.Errorf("finalVerdict not parsed with source-confirmed narrative: %q", got.FinalVerdict)
	}
	if len(got.CausalChain) != 3 || got.CausalChain[0].Timestamp == "" || got.CausalChain[0].Citation == "" {
		t.Errorf("causalChain not parsed with timestamp/citation: %+v", got.CausalChain)
	}
	var sawSymptom bool
	for _, s := range got.SymptomVsCause {
		if strings.Contains(s.Signal, "connection refused") && s.Classification == "symptom" {
			sawSymptom = true
		}
	}
	if !sawSymptom {
		t.Errorf("expected connection-refused labeled as symptom, got %+v", got.SymptomVsCause)
	}
	if got.Falsification == nil || got.Falsification.Outcome != "refuted" {
		t.Errorf("expected falsification outcome refuted, got %+v", got.Falsification)
	}
	if len(got.EvidenceGaps) != 1 || !strings.Contains(got.EvidenceGaps[0].HowToCapture, "--previous") {
		t.Errorf("expected an evidence gap with capture command, got %+v", got.EvidenceGaps)
	}
	if len(got.KnownUnknowns) != 1 {
		t.Errorf("expected one known-unknown, got %+v", got.KnownUnknowns)
	}
}

// TestLLMClassifierDropsEmptyFalsification verifies an all-empty falsification
// object is normalized to nil so the report does not render an empty section.
func TestLLMClassifierDropsEmptyFalsification(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "known_flake",
		"confidence": 0.5,
		"rootCauseSummary": "transient",
		"topEvidence": [],
		"causalChain": [],
		"symptomVsCause": [],
		"falsification": {"hypothesis": "", "ifTrueExpect": "", "ifFalseExpect": "", "correlationResult": "", "outcome": ""},
		"evidenceGaps": [],
		"knownUnknowns": [],
		"recommendedOwner": "acn-cni",
		"proposedFix": "retry",
		"nodeAssessment": "healthy"
	}`}
	got, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, PriorContext{})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Falsification != nil {
		t.Errorf("expected empty falsification to be dropped, got %+v", got.Falsification)
	}
}

func TestLLMClassifierInjectsPriorKnowledge(t *testing.T) {
	fc := &fakeCompleter{response: `{
		"category": "known_flake",
		"confidence": 0.7,
		"rootCauseSummary": "recurring image pull flake",
		"topEvidence": ["ImagePullBackOff"],
		"recommendedOwner": "acn-cni",
		"proposedFix": "retry"
	}`}
	prior := PriorContext{
		Resolved:   []PriorIncident{{Category: "known_flake", Summary: "same image pull flake", Fix: "bump retry budget", Status: "validated_resolved"}},
		Unresolved: []PriorIncident{{Category: "known_flake", Summary: "open flake report", Status: "pr_open"}},
	}

	if _, err := NewLLMClassifier(fc).Classify(context.Background(), model.RunContext{}, model.Evidence{}, model.Fingerprint{}, nil, prior); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !strings.Contains(fc.gotUser, "Prior validated resolutions") {
		t.Error("expected validated resolutions section in prompt")
	}
	if !strings.Contains(fc.gotUser, "bump retry budget") {
		t.Error("expected validated fix text in prompt")
	}
	if !strings.Contains(fc.gotUser, "NOT yet validated") {
		t.Error("expected in-flight incidents to be clearly labeled")
	}
}
