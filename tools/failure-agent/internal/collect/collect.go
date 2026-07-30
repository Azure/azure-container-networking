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

// maxExcerptFiles caps how many file excerpts are retained.
const maxExcerptFiles = 15

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

// ParseEvidence walks root and extracts error lines, file excerpts, and the
// file inventory. It is read-only and skips unreadable or non-text files.
func ParseEvidence(root string) (model.Evidence, error) {
	ev := model.Evidence{Root: root, Excerpts: map[string]string{}}

	seen := map[string]bool{}
	var errorLines []string

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
		for _, l := range lines {
			key := normalizeForDedup(l)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			errorLines = append(errorLines, l)
		}
		if len(snippets) == 0 && isNodeEvidenceFile(rel) {
			if head := headExcerpt(path); head != "" {
				snippets = []model.ErrorSnippet{{Line: 1, Snippet: head}}
			}
		}
		if len(snippets) > 0 {
			if len(ev.Excerpts) < maxExcerptFiles {
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
	if len(errorLines) > maxTopErrorLines {
		errorLines = errorLines[:maxTopErrorLines]
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
