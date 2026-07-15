package grounding

import (
	"regexp"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// versionEnvVars are pipeline variables that, when present, name a component
// version directly. Evidence scanning (below) covers everything else.
var versionEnvVars = map[string]string{
	"KUBERNETES_VERSION":  "kubernetes",
	"K8S_VERSION":         "kubernetes",
	"CILIUM_VERSION":      "cilium",
	"AKS_PREVIEW_VERSION": "aks-preview",
	"CNS_VERSION":         "azure-cns",
	"CNI_VERSION":         "azure-cni",
	"NPM_VERSION":         "azure-npm",
	"IPAM_VERSION":        "azure-ipam",
	"DOCKER_IMAGE_TAG":    "image-tag",
	"IMAGE_TAG":           "image-tag",
	"TAG":                 "image-tag",
}

// evidenceVersionREs extract component versions from collected log text. The
// first submatch is the value stored under the mapped component key.
var evidenceVersionREs = []struct {
	component string
	re        *regexp.Regexp
}{
	{"kubernetes", regexp.MustCompile(`(?i)server version:\s*v?(\d+\.\d+\.\d+[\w.\-]*)`)},
	{"kubernetes", regexp.MustCompile(`(?i)\bkubernetes\b[^\n]{0,40}?v(\d+\.\d+\.\d+)`)},
	{"cilium", regexp.MustCompile(`(?i)cilium[:/ \-]+v?(\d+\.\d+\.\d+[\w.\-]*)`)},
	{"azure-cns", regexp.MustCompile(`(?i)azure-cns[:@]([\w.\-]+)`)},
	{"azure-cni", regexp.MustCompile(`(?i)azure-cni[:@]([\w.\-]+)`)},
	{"azure-npm", regexp.MustCompile(`(?i)azure-npm[:@]([\w.\-]+)`)},
	{"azure-ipam", regexp.MustCompile(`(?i)azure-ipam[:@]([\w.\-]+)`)},
	{"cni-dropgz", regexp.MustCompile(`(?i)cni-dropgz[:@]([\w.\-]+)`)},
}

// DetectVersions gathers the component versions in effect for the run, first
// from the CI environment and then from the collected evidence text. Values are
// best-effort; absent components are simply omitted. Environment values take
// precedence over evidence-scraped ones.
func DetectVersions(getenv func(string) string, ev model.Evidence) map[string]string {
	versions := map[string]string{}

	scanEvidenceVersions(versions, ev)

	// Environment wins over evidence: overwrite any scraped value.
	for env, component := range versionEnvVars {
		if v := strings.TrimSpace(getenv(env)); v != "" {
			versions[component] = v
		}
	}
	return versions
}

func scanEvidenceVersions(versions map[string]string, ev model.Evidence) {
	var text strings.Builder
	for _, l := range ev.TopErrorLines {
		text.WriteString(l)
		text.WriteByte('\n')
	}
	for _, excerpt := range ev.Excerpts {
		text.WriteString(excerpt)
		text.WriteByte('\n')
	}
	blob := text.String()

	for _, ver := range evidenceVersionREs {
		if _, ok := versions[ver.component]; ok {
			continue // keep the first match per component
		}
		if m := ver.re.FindStringSubmatch(blob); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			versions[ver.component] = strings.TrimSpace(m[1])
		}
	}
}

// BaseRef derives the base ref to diff against from the CI environment. For a
// pull-request build it is the target branch (normalized to origin/<branch>);
// otherwise it defaults to origin/master.
func BaseRef(getenv func(string) string) string {
	if target := getenv("SYSTEM_PULLREQUEST_TARGETBRANCH"); target != "" {
		return "origin/" + strings.TrimPrefix(target, "refs/heads/")
	}
	return "origin/master"
}

// HeadRef derives the head ref (tip of the change) from the CI environment,
// preferring the PR source commit and falling back to HEAD.
func HeadRef(getenv func(string) string) string {
	if c := getenv("SYSTEM_PULLREQUEST_SOURCECOMMITID"); c != "" {
		return c
	}
	return "HEAD"
}
