// Package collect gathers the run context from the CI environment and parses
// the downloaded failure-log bundle into structured evidence.
package collect

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// maxFileBytes caps how much of any single evidence file is scanned.
const maxFileBytes = 5 << 20 // 5 MiB

// maxExcerptBytes caps the stored excerpt per interesting file.
const maxExcerptBytes = 2 << 10 // 2 KiB

// maxTopErrorLines caps how many distinct error lines are retained.
const maxTopErrorLines = 25

// maxExcerptFiles caps how many file excerpts are retained. It is set above the
// error-excerpt working set so datapath/IP-plane state dumps (which carry no
// error keywords) can be surfaced as head excerpts without crowding out the
// error excerpts that drive the primary root-cause read.
const maxExcerptFiles = 24

// maxSnippetsPerFile caps how many context snippets are retained per file.
const maxSnippetsPerFile = 3

// snippetContextLines controls how many lines before/after a match are included.
const snippetContextLines = 3

// maxErrorSnippets caps the number of snippets retained across all files.
const maxErrorSnippets = 30

// errorLineRE matches lines that look like failures worth surfacing.
var errorLineRE = regexp.MustCompile(`(?i)\b(error|fatal|fail(ed|ure)?|panic|timed?\s*out|timeout|exceeded|refused|cannot|unable to|denied|not found|crashloopbackoff|imagepullbackoff|oomkilled)\b`)

// textExtensions are the file extensions parsed for evidence. Files without an
// extension are also parsed (CI logs are frequently extension-less).
var textExtensions = map[string]bool{
	".txt": true, ".log": true, ".out": true, ".json": true,
	".yaml": true, ".yml": true, ".md": true, ".err": true,
}

// FromEnv builds a RunContext from the CI environment. getenv is injected so
// the function is testable without mutating the process environment.
func FromEnv(getenv func(string) string) model.RunContext {
	rc := model.RunContext{
		PipelineName:      getenv("BUILD_DEFINITIONNAME"),
		BuildID:           getenv("BUILD_BUILDID"),
		BuildNumber:       getenv("BUILD_BUILDNUMBER"),
		Repository:        getenv("BUILD_REPOSITORY_NAME"),
		StageName:         firstNonEmpty(getenv("SYSTEM_STAGEDISPLAYNAME"), getenv("SYSTEM_STAGENAME")),
		JobName:           firstNonEmpty(getenv("SYSTEM_JOBDISPLAYNAME"), getenv("SYSTEM_JOBNAME")),
		PullRequestNumber: getenv("SYSTEM_PULLREQUEST_PULLREQUESTNUMBER"),
		SourceBranch:      getenv("SYSTEM_PULLREQUEST_SOURCEBRANCH"),
		TargetBranch:      getenv("SYSTEM_PULLREQUEST_TARGETBRANCH"),
		SourceCommitID:    getenv("SYSTEM_PULLREQUEST_SOURCECOMMITID"),
		CommitID:          firstNonEmpty(getenv("commitID"), getenv("BUILD_SOURCEVERSION")),
	}
	rc.IsPR = strings.EqualFold(getenv("BUILD_REASON"), "PullRequest") || rc.PullRequestNumber != ""
	return rc
}

// maxDiffChars bounds the change-under-test diff injected into the prompt so a
// large pull request cannot crowd collected evidence out of the excerpt budget.
const maxDiffChars = 60 << 10 // 60 KiB

// diffPostImageRE matches the "+++ b/<path>" line of a unified diff, which names
// the post-image path of each changed file.
var diffPostImageRE = regexp.MustCompile(`(?m)^\+\+\+ b/(.+)$`)

// LoadChangeContext reads a unified diff of the change under test and populates
// rc.ChangedFiles and rc.Diff. An empty diff is not an error: the agent still
// analyzes the run, just without change context.
func LoadChangeContext(rc *model.RunContext, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading diff %s: %w", path, err)
	}

	diff := string(b)
	if strings.TrimSpace(diff) == "" {
		return nil
	}

	seen := map[string]bool{}
	for _, m := range diffPostImageRE.FindAllStringSubmatch(diff, -1) {
		name := strings.TrimSpace(m[1])
		if name == "" || name == "/dev/null" || seen[name] {
			continue
		}
		seen[name] = true
		rc.ChangedFiles = append(rc.ChangedFiles, name)
	}
	sort.Strings(rc.ChangedFiles)

	if len(diff) > maxDiffChars {
		diff = diff[:maxDiffChars] + "\n... (diff truncated)\n"
	}
	rc.Diff = diff
	return nil
}

// ParseEvidence walks root and extracts error lines, file excerpts, and the
// file inventory. It is read-only and skips unreadable or non-text files.
func ParseEvidence(root string) (model.Evidence, error) {
	ev := model.Evidence{Root: root, Excerpts: map[string]string{}}

	seen := map[string]bool{}
	type fileErrors struct {
		rel   string
		lines []string
	}
	var perFile []fileErrors

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if d.IsDir() || !isTextFile(d.Name()) {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil || info.Size() == 0 {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		ev.Files = append(ev.Files, rel)

		lines, snippets := scanFile(path)
		var kept []string
		for _, l := range lines {
			key := normalizeForDedup(l)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			kept = append(kept, l)
		}
		if len(kept) > 0 {
			perFile = append(perFile, fileErrors{rel: rel, lines: kept})
		}
		if len(snippets) == 0 && (isNodeEvidenceFile(rel) || isDatapathEvidenceFile(rel)) {
			if head := headExcerpt(path); head != "" {
				snippets = []model.ErrorSnippet{{Line: 1, Snippet: head}}
			}
		}
		if len(snippets) > 0 {
			// High-signal files bypass the excerpt cap: the walk is lexical, so
			// per-node directories are visited first and would otherwise consume
			// every slot before the assertion and IP-plane state are reached.
			if len(ev.Excerpts) < maxExcerptFiles || isAssertionEvidenceFile(rel) || isDatapathEvidenceFile(rel) {
				ev.Excerpts[rel] = renderFileExcerpt(snippets)
			}
			for _, sn := range snippets {
				if len(ev.ErrorSnippets) >= maxErrorSnippets {
					break
				}
				sn.File = rel
				ev.ErrorSnippets = append(ev.ErrorSnippets, sn)
			}
		}
		return nil
	})
	if walkErr != nil {
		return ev, walkErr
	}

	sort.Strings(ev.Files)

	// Draw error lines round-robin across files rather than in walk order. A
	// single verbose source (containerd's journal is 1000 lines and sorts before
	// the CNS log) would otherwise fill the whole budget before the decisive
	// files are read.
	var errorLines []string
	for i := 0; len(errorLines) < maxTopErrorLines; i++ {
		progressed := false
		for _, fe := range perFile {
			if i >= len(fe.lines) {
				continue
			}
			progressed = true
			errorLines = append(errorLines, fe.lines[i])
			if len(errorLines) >= maxTopErrorLines {
				break
			}
		}
		if !progressed {
			break
		}
	}
	ev.TopErrorLines = errorLines
	return ev, nil
}

// nodeEvidenceNameRE matches evidence files that describe node/nodepool health.
// These are surfaced as excerpts even when they contain no error keywords, since
// node readiness/lifecycle signals (NotReady, reboot, pressure) do not match the
// error regex but are essential to distinguish infra failures from PR regressions.
var nodeEvidenceNameRE = regexp.MustCompile(`(?i)(^|/)(node-status|node-conditions|node-network|nodes)[a-z0-9-]*(\.[a-z]+)?$`)

// isNodeEvidenceFile reports whether rel is a node/nodepool health file.
func isNodeEvidenceFile(rel string) bool {
	return nodeEvidenceNameRE.MatchString(rel)
}

// datapathEvidenceNameRE matches high-signal IP-plane / datapath state dumps.
// Like node-health files, these describe *state* (IP allocation, endpoints,
// routes, VFP policy) rather than errors, so they never match errorLineRE and
// must be surfaced explicitly. The allowlist is deliberately narrow: it targets
// the CNS/CNI IPAM view (azure-cns, cnsCache, azure-endpoints), the Windows
// dataplane (hns-endpoint, hns-network) and the core extracted network dumps
// (endpoint, routes, ports, vfpOutput, ip) and the cluster-wide pod/IP table
// (all-pods), while excluding low-signal noise (per-adapter interface dumps,
// arp, firewall, dism/cbs) that would otherwise exhaust the excerpt budget.
// all-pods is included because CNS state and the orchestrator's own view of pod
// addressing are indexed differently, so reconciling the two requires the pod
// table as well as the CNS dumps.
var datapathEvidenceNameRE = regexp.MustCompile(`(?i)(^|/)(azure-cns|azure-vnet|cnscache|azure-endpoints|hns-endpoint|hns-network|endpoint|routes|ports|vfpoutput|ip|all-pods)(\.[a-z]+)?$`)

// isDatapathEvidenceFile reports whether rel is an IP-plane/datapath state file.
func isDatapathEvidenceFile(rel string) bool {
	return datapathEvidenceNameRE.MatchString(rel)
}

// assertionEvidenceNameRE matches the E2E task logs pulled from the ADO build
// timeline. They carry the test's own assertion text, which is
// written to the task log and to no file on disk, so it exists in no
// cluster-side artifact.
var assertionEvidenceNameRE = regexp.MustCompile(`(?i)(^|/)e2e-task-logs/`)

// isAssertionEvidenceFile reports whether rel is a captured E2E task log.
func isAssertionEvidenceFile(rel string) bool {
	return assertionEvidenceNameRE.MatchString(rel)
}

// headExcerpt returns the first lines of a file as a line-numbered snippet, used
// to surface node-health files that carry no error-keyword matches.
func headExcerpt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(&boundedReader{r: f, remaining: maxExcerptBytes})
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var b strings.Builder
	lineNo := 0
	for scanner.Scan() && b.Len() < maxExcerptBytes {
		lineNo++
		fmt.Fprintf(&b, "  %6d | %s\n", lineNo, strings.TrimSpace(scanner.Text()))
	}
	return strings.TrimSpace(b.String())
}

// scanFile returns matched error lines and line-numbered context snippets.
func scanFile(path string) (lines []string, snippets []model.ErrorSnippet) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(&boundedReader{r: f, remaining: maxFileBytes})
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	var allLines []string
	var matchLines []int
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		allLines = append(allLines, line)
		if errorLineRE.MatchString(line) {
			matchLines = append(matchLines, lineNo)
			lines = append(lines, line)
		}
	}
	if len(matchLines) == 0 {
		return nil, nil
	}
	for i, matched := range matchLines {
		if i >= maxSnippetsPerFile {
			break
		}
		snippets = append(snippets, model.ErrorSnippet{
			Line:    matched,
			Snippet: renderContextSnippet(allLines, matched),
		})
	}
	return lines, snippets
}

func renderFileExcerpt(snippets []model.ErrorSnippet) string {
	var b strings.Builder
	for i, sn := range snippets {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		if b.Len() >= maxExcerptBytes {
			break
		}
		fmt.Fprintf(&b, "match line %d\n%s", sn.Line, sn.Snippet)
	}
	out := b.String()
	if len(out) > maxExcerptBytes {
		out = out[:maxExcerptBytes]
	}
	return strings.TrimSpace(out)
}

func renderContextSnippet(lines []string, matchedLine int) string {
	start := matchedLine - snippetContextLines
	if start < 1 {
		start = 1
	}
	end := matchedLine + snippetContextLines
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := start; i <= end; i++ {
		marker := " "
		if i == matchedLine {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %6d | %s\n", marker, i, lines[i-1])
	}
	return strings.TrimSpace(b.String())
}

func isTextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return true
	}
	return textExtensions[ext]
}

var dedupNoiseRE = regexp.MustCompile(`\s+`)

// normalizeForDedup collapses whitespace and lowercases so near-identical
// lines are deduplicated without losing the original text.
func normalizeForDedup(s string) string {
	return strings.ToLower(dedupNoiseRE.ReplaceAllString(strings.TrimSpace(s), " "))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// boundedReader limits the number of bytes read from the underlying reader.
type boundedReader struct {
	r         io.Reader
	remaining int
}

func (b *boundedReader) Read(p []byte) (int, error) {
	if b.remaining <= 0 {
		return 0, io.EOF
	}
	if len(p) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.r.Read(p)
	b.remaining -= n
	return n, err
}
