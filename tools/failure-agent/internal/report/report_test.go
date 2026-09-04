package report

import (
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

func sampleClassification() model.Classification {
	return model.Classification{
		Category:         model.CategoryUnknownNeedsHuman,
		Confidence:       0,
		RootCauseSummary: "Unclassified failure.",
		TopEvidence:      []string{"some error line"},
		Source:           "llm",
	}
}

func TestRenderMarkdownNodeAssessment(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"},
		model.Classification{
			Category:         model.CategoryPipelineInfraConfig,
			Confidence:       0.6,
			RootCauseSummary: "CNS restarted after the node rebooted.",
			NodeAssessment:   "Node aks-nodepool1-vmss000000 went NotReady after a RebootScheduled event; CNS restart is a side effect.",
			Source:           "llm",
		}, nil, model.Evidence{})

	md := RenderMarkdown(inc)
	if !strings.Contains(md, "### Node / nodepool health") {
		t.Error("expected node/nodepool health section in report")
	}
	if !strings.Contains(md, "RebootScheduled") {
		t.Error("expected node assessment text in report")
	}
}

func TestRenderMarkdownOmitsEmptyNodeAssessment(t *testing.T) {
	md := RenderMarkdown(Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{}))
	if strings.Contains(md, "Node / nodepool health") {
		t.Error("did not expect node section when assessment is empty")
	}
}

func TestBuildAppliesPolicy(t *testing.T) {
	rc := model.RunContext{PipelineName: "ACN", StageName: "Cilium", SourceCommitID: "abc123"}
	inc := Build(time.Unix(0, 0), rc, model.Fingerprint{Hash: "deadbeef"}, sampleClassification(), nil, model.Evidence{})

	if inc.ConfidenceBand != model.BandLow {
		t.Errorf("band: got %s", inc.ConfidenceBand)
	}
	if inc.RetentionDecision != model.RetentionRetainTTL {
		t.Errorf("retention: got %s", inc.RetentionDecision)
	}
	if inc.Commit != "abc123" {
		t.Errorf("commit: got %q", inc.Commit)
	}
	if inc.Fingerprint != "deadbeef" {
		t.Errorf("fingerprint: got %q", inc.Fingerprint)
	}
}

func TestBuildDefaultsToAnalyzed(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{})
	if inc.AnalysisStatus != model.StatusAnalyzed {
		t.Errorf("status: got %s, want analyzed", inc.AnalysisStatus)
	}
}

func TestRenderMarkdownShowsAnalysisFailedBanner(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{})
	inc.AnalysisStatus = model.StatusAnalysisFailed
	inc.AnalysisError = "azure openai unauthorized"
	md := RenderMarkdown(inc)

	if !strings.Contains(md, "Automated analysis failed") {
		t.Errorf("expected analysis-failed banner, got:\n%s", md)
	}
	if !strings.Contains(md, "azure openai unauthorized") {
		t.Error("expected analysis error reason in markdown")
	}
}

func TestRenderMarkdownRendersContractSections(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{PipelineName: "ACN"}, model.Fingerprint{Hash: "deadbeef"},
		model.Classification{
			Category:         model.CategoryPipelineInfraConfig,
			Confidence:       0.72,
			RootCauseSummary: "Defender init container install-packages.sh exited 1 and crashlooped.",
			FinalVerdict: "Root cause - confirmed from the pods' own embedded init script\n\n" +
				"The describe dump embeds the exact exit path:\n\n" +
				"```text\n" +
				"1044 emit_k8s_event \"Warning\" \"PackageInstallFailed\" \"$_msg\"\n" +
				"1045 echo \"ERROR: install-packages.sh failed with exit code $install_rc\"\n" +
				"1048 exit $install_rc\n" +
				"```\n\n" +
				"| Pod | Stage | Image |\n|---|---|---|\n| 96h8w | Podsubnet | same |\n\n" +
				"Verdict: AzSecPack/node-image infra, not an ACN/CNS regression. Capture `kubectl logs <pod> -c init-package-installer --previous` before event TTL.",
			TopAnomaly:       "kube-system Defender init container CrashLoopBackOff x289 over 75m",
			FailingUnit:      "install-packages.sh in the azsecpack init-package-installer container",
			RecommendedOwner: "aks-node-image",
			CausalChain: []model.CausalHop{
				{Step: "Nodes rebooted", Timestamp: "2026-07-27T08:59:00Z", Citation: "node-conditions.txt"},
				{Step: "install-packages.sh exited 1", Citation: "bad-pod-describe.txt line 1048"},
			},
			SymptomVsCause: []model.SymptomCause{
				{Signal: "CNS connection refused", Classification: "symptom", Justification: "CNS restarted during the node disruption"},
				{Signal: "install-packages.sh exit 1", Classification: "cause", Justification: "primary init failure"},
			},
			Falsification: &model.Falsification{
				Hypothesis:        "CNS IPAM regression",
				CorrelationResult: "identical signature across 2 nodes and 2 stages, same commit and image",
				Outcome:           "refuted",
			},
			EvidenceGaps: []model.EvidenceGap{
				{Missing: "literal dpkg error", WhereItLives: "expired k8s event", WhyMissing: "events ~1h TTL", HowToCapture: "kubectl logs <pod> -c init-package-installer --previous"},
			},
			KnownUnknowns: []string{"exact failing .deb postinst step not captured"},
			RootCauseSources: []model.RootCauseRef{
				{
					File:        "bad-pod-describe.txt",
					Line:        1048,
					EndLine:     1048,
					Snippet:     "  1045 | echo \"ERROR: install-packages.sh failed with exit code $install_rc\"\n> 1048 | exit $install_rc",
					Explanation: "the init script exits with the installer's non-zero code, crashlooping the pod",
				},
			},
			Source: "llm",
		}, nil, model.Evidence{})

	md := RenderMarkdown(inc)
	for _, want := range []string{
		"### Final verdict",
		"Root cause - confirmed from the pods' own embedded init script",
		"```text",
		"| Pod | Stage | Image |",
		"### Most severe anomaly",
		"### Root cause source",
		"**bad-pod-describe.txt:1048**",
		"exit $install_rc",
		"### Causal chain",
		"### Symptom vs cause",
		"### Falsification",
		"### Evidence gaps",
		"### Known-unknowns",
		"**Failing unit:**",
		"connection refused",
		"refuted",
		"--previous",
		"owns the failing unit",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in markdown, got:\n%s", want, md)
		}
	}
	finalIdx := strings.Index(md, "### Final verdict")
	whereIdx := strings.Index(md, "### Where")
	if finalIdx == -1 || whereIdx == -1 || finalIdx > whereIdx {
		t.Errorf("expected final verdict before Where, got:\n%s", md)
	}
}

// TestRenderMarkdownRendersBothRootCauseSourceClasses verifies the report cites
// the observed symptom (a collected artifact) and the responsible code (a hunk
// from the change under test) side by side, rather than one in place of the other.
func TestRenderMarkdownRendersBothRootCauseSourceClasses(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"},
		model.Classification{
			Category:         model.CategoryPRRegression,
			Confidence:       0.85,
			RootCauseSummary: "The pod-IP route to the host veth is no longer programmed.",
			RootCauseSources: []model.RootCauseRef{
				{
					File:        "network/transparent_endpointclient_linux.go",
					Line:        157,
					EndLine:     165,
					Snippet:     "@@ -157,9 +157,6 @@\n-\tif err := addRoutes(client.netlink, client.netioshim, client.hostVethName, routeInfoList); err != nil {\n-\t\treturn newErrorTransparentEndpointClient(err)\n-\t}",
					Explanation: "deleted lines: the pod-IP route to hostVeth is never added, so inbound packets have no path",
				},
				{
					File:        "aks-node-1/routes.txt",
					Line:        37,
					EndLine:     37,
					Snippet:     "> 37 | 10.224.0.5 dev azv1a2b3c scope link",
					Explanation: "no route exists for the failing pod IP on any host veth",
				},
			},
			Source: "llm",
		}, nil, model.Evidence{})

	md := RenderMarkdown(inc)
	for _, want := range []string{
		"### Root cause source",
		"**network/transparent_endpointclient_linux.go:157-165**",
		"@@ -157,9 +157,6 @@",
		"**aks-node-1/routes.txt:37**",
		"10.224.0.5 dev azv1a2b3c scope link",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in markdown, got:\n%s", want, md)
		}
	}
}

func TestRenderMarkdownOmitsContractSectionsWhenEmpty(t *testing.T) {
	md := RenderMarkdown(Build(time.Unix(0, 0), model.RunContext{}, model.Fingerprint{Hash: "x"}, sampleClassification(), nil, model.Evidence{}))
	for _, unwanted := range []string{"### Final verdict", "### Causal chain", "### Symptom vs cause", "### Falsification", "### Evidence gaps", "### Known-unknowns", "### Most severe anomaly", "### Root cause source"} {
		if strings.Contains(md, unwanted) {
			t.Errorf("did not expect %q when contract fields are empty", unwanted)
		}
	}
}

func TestRenderMarkdownContainsMarkerAndFields(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{PipelineName: "ACN"}, model.Fingerprint{Hash: "deadbeef"}, sampleClassification(), nil, model.Evidence{})
	md := RenderMarkdown(inc)

	if !strings.HasPrefix(md, CommentMarker("deadbeef")) {
		t.Errorf("expected marker as first line, got:\n%s", md)
	}
	for _, want := range []string{"ACN Pipeline Failure Analysis", "unknown_needs_human", "Recommended next action"} {
		if !strings.Contains(md, want) {
			t.Errorf("expected %q in markdown", want)
		}
	}
}

// TestRenderMarkdownRendersEscalation covers every gate outcome. A decline and a
// gate failure are rendered too: silence would make them indistinguishable from
// each other and from a gate that never ran.
func TestRenderMarkdownRendersEscalation(t *testing.T) {
	tests := []struct {
		name       string
		escalation *model.Escalation
		want       []string
		unwanted   []string
	}{
		{
			name:     "absent gate renders no section",
			unwanted: []string{"### GitHub issue escalation"},
		},
		{
			name: "escalated",
			escalation: &model.Escalation{
				Needed:       true,
				Reason:       "The bad tag is committed here.",
				Title:        "CNS image tag does not exist",
				Labels:       []string{"bug", "cns"},
				FixDirection: "Point the template at a published tag.",
				Source:       model.EscalationLLM,
			},
			want: []string{
				"### GitHub issue escalation",
				"A GitHub issue is warranted",
				"issue.md",
				"The bad tag is committed here.",
				"CNS image tag does not exist",
				"`bug`, `cns`",
				"Point the template at a published tag.",
			},
		},
		{
			name: "declined keeps the reason but drafts nothing",
			escalation: &model.Escalation{
				Reason: "Quota exhaustion is not fixable in code.",
				Source: model.EscalationLLM,
			},
			want:     []string{"No GitHub issue raised", "Quota exhaustion is not fixable in code."},
			unwanted: []string{"Proposed title", "Fix direction"},
		},
		{
			name:       "skipped",
			escalation: &model.Escalation{Reason: "Pull-request build.", Source: model.EscalationSkipped},
			want:       []string{"Not evaluated for this run", "Pull-request build."},
		},
		{
			name:       "gate failure is surfaced, not silent",
			escalation: &model.Escalation{Reason: "azure openai unauthorized", Source: model.EscalationError},
			want:       []string{"Escalation gate failed", "azure openai unauthorized"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inc := Build(time.Unix(0, 0), model.RunContext{PipelineName: "ACN"}, model.Fingerprint{Hash: "deadbeef"}, sampleClassification(), nil, model.Evidence{})
			inc.Escalation = tt.escalation
			md := RenderMarkdown(inc)

			for _, want := range tt.want {
				if !strings.Contains(md, want) {
					t.Errorf("expected %q in markdown, got:\n%s", want, md)
				}
			}
			for _, unwanted := range tt.unwanted {
				if strings.Contains(md, unwanted) {
					t.Errorf("did not expect %q in markdown", unwanted)
				}
			}
		})
	}
}

// TestRenderMarkdownEscalationReasonStaysOnOneLine keeps a multi-line model
// reason from breaking the surrounding markdown.
func TestRenderMarkdownEscalationReasonStaysOnOneLine(t *testing.T) {
	inc := Build(time.Unix(0, 0), model.RunContext{PipelineName: "ACN"}, model.Fingerprint{Hash: "deadbeef"}, sampleClassification(), nil, model.Evidence{})
	inc.Escalation = &model.Escalation{Reason: "first\nsecond", Source: model.EscalationLLM}

	md := RenderMarkdown(inc)
	if !strings.Contains(md, "first second") {
		t.Errorf("expected the reason collapsed to one line, got:\n%s", md)
	}
}
