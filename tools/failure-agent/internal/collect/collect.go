// Package collect gathers the run context from the CI environment and parses
// the downloaded failure-log bundle into structured evidence.
package collect

import (
	"bufio"
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

		lines, excerpt := scanFile(path)
		for _, l := range lines {
			key := normalizeForDedup(l)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			errorLines = append(errorLines, l)
		}
		if excerpt != "" && len(ev.Excerpts) < maxExcerptFiles {
			ev.Excerpts[rel] = excerpt
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

// scanFile returns the error lines and a leading excerpt from a single file.
func scanFile(path string) (lines []string, excerpt string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ""
	}
	defer f.Close()

	var excerptB strings.Builder
	scanner := bufio.NewScanner(&boundedReader{r: f, remaining: maxFileBytes})
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)

	hasMatch := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if excerptB.Len() < maxExcerptBytes {
			excerptB.WriteString(line)
			excerptB.WriteByte('\n')
		}
		if errorLineRE.MatchString(line) {
			hasMatch = true
			lines = append(lines, line)
		}
	}
	if !hasMatch {
		return nil, ""
	}
	return lines, strings.TrimSpace(excerptB.String())
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
