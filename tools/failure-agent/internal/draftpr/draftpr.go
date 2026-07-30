// Package draftpr dispatches a 1ES Agency coding job that authors a fix for a
// non-PR build failure and opens a draft pull request. The failure-analysis
// agent supplies a grounded prompt; the Agency coding-agent branches, writes the
// code, and opens the draft PR inside Azure DevOps.
package draftpr

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/publish"
)

const (
	// PluginAgencyCoding is the Agency plugin that brings the coding skill and
	// build tools authenticated as the calling service principal.
	PluginAgencyCoding = "agency-coding"
	// CodingAgent is the bundled agent that must drive the job; the default agent
	// may otherwise make zero build-tool calls (see the Gardener scenario docs).
	CodingAgent = "coding-agent"
)

// Client dispatches Agency v2 jobs.
type Client interface {
	CreateV2Job(ctx context.Context, req publish.V2JobRequest) (publish.JobResponse, error)
}

// Params are the Azure DevOps coordinates and prompt for a draft-PR job.
type Params struct {
	Organization string
	Project      string
	Repository   string
	Branch       string
	Prompt       string
	Fingerprint  string
}

// Result summarizes a dispatched job.
type Result struct {
	JobID string
	State string
}

// ShouldOpen reports whether a draft-PR job should be dispatched for this run.
// Draft PRs are only opened for non-PR build failures that the LLM analyzed with
// high confidence as a regression in the change under test, and for which it
// produced a proposed fix to ground the prompt. PR builds stay comment-only.
func ShouldOpen(rc model.RunContext, inc model.Incident) bool {
	return !rc.IsPR &&
		inc.AnalysisStatus == model.StatusAnalyzed &&
		inc.Category == model.CategoryPRRegression &&
		inc.ConfidenceBand == model.BandHigh &&
		strings.TrimSpace(inc.ProposedFix) != ""
}

// Open dispatches the coding job and returns the resulting job identity.
func Open(ctx context.Context, client Client, p Params) (Result, error) {
	if p.Organization == "" || p.Project == "" || p.Repository == "" {
		return Result{}, fmt.Errorf("agency org, project, and repository are required")
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return Result{}, fmt.Errorf("a non-empty prompt is required")
	}

	req := publish.V2JobRequest{
		Prompt: p.Prompt,
		Context: publish.JobContext{
			Repository: publish.ADORepository{
				Type:         publish.RepoTypeADO,
				Organization: p.Organization,
				Project:      p.Project,
				Repository:   p.Repository,
				Branch:       p.Branch,
			},
		},
		CustomAgent: &publish.AgentRef{
			Name:       CodingAgent,
			Type:       publish.AgentTypeCompany,
			PluginName: PluginAgencyCoding,
		},
		Plugins: []publish.PluginRef{
			{Name: PluginAgencyCoding, Type: publish.AgentTypeCompany},
		},
		Metadata: map[string]string{
			"source":         "failure-analysis-agent",
			"faaFingerprint": p.Fingerprint,
		},
	}

	resp, err := client.CreateV2Job(ctx, req)
	if err != nil {
		return Result{}, err
	}
	return Result{JobID: resp.JobID, State: resp.State}, nil
}

// BuildPrompt renders the grounded instruction for the coding-agent from the
// incident analysis. It asks the agent to open the pull request as a draft.
func BuildPrompt(inc model.Incident) string {
	var b strings.Builder
	b.WriteString("A non-PR Azure DevOps pipeline build failed and was analyzed as a code regression. ")
	b.WriteString("Author the smallest correct fix and open it as a DRAFT pull request for human review. ")
	b.WriteString("Do not mark the pull request ready for review.\n\n")

	writeField(&b, "Pipeline", inc.PipelineName)
	writeField(&b, "Stage", inc.Stage)
	writeField(&b, "Job", inc.Job)
	writeField(&b, "Failing commit", inc.Commit)
	writeField(&b, "Fingerprint", inc.Fingerprint)
	writeField(&b, "Root cause", inc.RootCauseSummary)
	writeField(&b, "Proposed fix", inc.ProposedFix)
	writeField(&b, "Recommended action", inc.RecommendedAction)
	writeField(&b, "Node assessment", inc.NodeAssessment)

	if len(inc.TopEvidence) > 0 {
		b.WriteString("\nTop evidence:\n")
		for _, e := range inc.TopEvidence {
			if strings.TrimSpace(e) == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(e))
		}
	}
	return strings.TrimSpace(b.String())
}

func writeField(b *strings.Builder, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s: %s\n", label, value)
}

// BaseBranchFromRef strips the refs/heads/ prefix from a build source branch.
func BaseBranchFromRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

// OrgFromCollectionURI extracts the Azure DevOps organization from
// SYSTEM_COLLECTIONURI, handling both dev.azure.com/{org} and
// {org}.visualstudio.com forms. It returns "" when the URI is unparseable.
func OrgFromCollectionURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	if host := u.Hostname(); strings.HasSuffix(host, ".visualstudio.com") {
		return strings.TrimSuffix(host, ".visualstudio.com")
	}
	if seg := strings.Split(strings.Trim(u.Path, "/"), "/"); len(seg) > 0 {
		return seg[0]
	}
	return ""
}
