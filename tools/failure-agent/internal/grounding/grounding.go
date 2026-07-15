// Package grounding gathers the extra context that turns a generic diagnosis
// into a specific, actionable one: the change under test (diff against the base
// branch, commits, changed-file inventory, and capped source excerpts) and the
// component versions in effect for the run.
//
// It shells out to git in the repo checkout and is strictly best-effort: any
// git failure or unavailable ref yields an empty or partial result rather than
// an error, so failure analysis always proceeds.
package grounding

import (
	"context"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// Caps bound how much change context is gathered so the prompt stays within a
// predictable budget regardless of how large the change under test is.
const (
	maxChangedFiles       = 60
	maxDiffBytesPerFile   = 6000
	maxDiffTotalBytes     = 20000
	maxCommits            = 30
	maxSourceExcerptFiles = 6
	maxSourceExcerptBytes = 4000
)

// GitRunner runs a git subcommand and returns its combined output. It is
// injected so Collect is testable without a real repository.
type GitRunner func(ctx context.Context, args ...string) (string, error)

// NewGitRunner returns a GitRunner that shells out to `git -C repoRoot`.
func NewGitRunner(repoRoot string) GitRunner {
	return func(ctx context.Context, args ...string) (string, error) {
		full := append([]string{"-C", repoRoot}, args...)
		out, err := exec.CommandContext(ctx, "git", full...).CombinedOutput()
		return string(out), err
	}
}

// Options controls change-context collection.
type Options struct {
	// BaseRef is the ref the change diverged from (e.g. origin/master or the PR
	// target branch). Collection is a no-op when it is empty.
	BaseRef string
	// HeadRef is the tip of the change (defaults to HEAD when empty).
	HeadRef string
	// Priority lists changed-file paths implicated by the failure; they are
	// surfaced first in the diff and chosen first for source excerpts.
	Priority []string
	// SourceExcerpts includes capped HEAD content of changed source files.
	SourceExcerpts bool
}

// Collect gathers the change under test between BaseRef and HeadRef. The result
// is best-effort: fields are populated only for the git queries that succeed.
func Collect(ctx context.Context, opts Options, git GitRunner) model.CodeContext {
	head := opts.HeadRef
	if head == "" {
		head = "HEAD"
	}
	cc := model.CodeContext{BaseRef: opts.BaseRef, HeadRef: head}
	if git == nil || strings.TrimSpace(opts.BaseRef) == "" {
		return cc
	}

	// Three-dot compares against the merge-base, i.e. only what the branch
	// changed since it diverged from the base — the true "diff from master".
	diffRange := opts.BaseRef + "..." + head

	out, err := git(ctx, "diff", "--name-only", diffRange)
	if err != nil {
		return cc
	}
	cc.ChangedFiles = capStrings(splitNonEmpty(out), maxChangedFiles)
	if len(cc.ChangedFiles) == 0 {
		return cc
	}

	ordered := prioritize(cc.ChangedFiles, opts.Priority)

	if stat, statErr := git(ctx, "diff", "--stat", diffRange); statErr == nil {
		cc.DiffStat = strings.TrimSpace(stat)
	}

	cc.Diff = collectDiff(ctx, git, diffRange, ordered)
	// Two-dot lists the commits unique to head (the ones that produced the change).
	cc.Commits = collectCommits(ctx, git, opts.BaseRef+".."+head)

	if opts.SourceExcerpts {
		cc.SourceExcerpts = collectSourceExcerpts(ctx, git, head, ordered)
	}
	return cc
}

func collectDiff(ctx context.Context, git GitRunner, diffRange string, files []string) string {
	var b strings.Builder
	total := 0
	for _, f := range files {
		if total >= maxDiffTotalBytes {
			break
		}
		out, err := git(ctx, "diff", diffRange, "--", f)
		if err != nil || strings.TrimSpace(out) == "" {
			continue
		}
		chunk := out
		if len(chunk) > maxDiffBytesPerFile {
			chunk = chunk[:maxDiffBytesPerFile] + "\n... [diff truncated]\n"
		}
		if remaining := maxDiffTotalBytes - total; len(chunk) > remaining {
			chunk = chunk[:remaining] + "\n... [diff budget exhausted]\n"
		}
		b.WriteString(chunk)
		total += len(chunk)
	}
	return strings.TrimSpace(b.String())
}

// commitFieldSep separates the SHA and subject in the git log format string. It
// is an ASCII unit separator so it never collides with commit-message text.
const commitFieldSep = "\x1f"

func collectCommits(ctx context.Context, git GitRunner, logRange string) []model.CommitMeta {
	out, err := git(ctx, "log", "--no-merges", "--format=%H"+commitFieldSep+"%s", logRange)
	if err != nil {
		return nil
	}
	var commits []model.CommitMeta
	for _, line := range splitNonEmpty(out) {
		sha, subject, ok := strings.Cut(line, commitFieldSep)
		if !ok {
			continue
		}
		commits = append(commits, model.CommitMeta{SHA: strings.TrimSpace(sha), Subject: strings.TrimSpace(subject)})
		if len(commits) >= maxCommits {
			break
		}
	}
	return commits
}

func collectSourceExcerpts(ctx context.Context, git GitRunner, head string, files []string) map[string]string {
	excerpts := map[string]string{}
	for _, f := range files {
		if len(excerpts) >= maxSourceExcerptFiles {
			break
		}
		if !isSourceFile(f) {
			continue
		}
		out, err := git(ctx, "show", head+":"+f)
		if err != nil || strings.TrimSpace(out) == "" {
			continue
		}
		if len(out) > maxSourceExcerptBytes {
			out = out[:maxSourceExcerptBytes] + "\n... [file truncated]\n"
		}
		excerpts[f] = out
	}
	return excerpts
}

// sourcePathRE extracts source-file-looking tokens from free text (error lines).
var sourcePathRE = regexp.MustCompile(`[\w./-]+\.(?:go|ya?ml|sh|py|json|tf|bicep)`)

// CandidatePathsFromLines extracts source-file paths referenced in log lines so
// the changed files they implicate can be surfaced first for the model.
func CandidatePathsFromLines(lines []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lines {
		for _, m := range sourcePathRE.FindAllString(l, -1) {
			m = strings.TrimLeft(m, "./")
			if m == "" || seen[m] {
				continue
			}
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// prioritize returns files with those matching a priority path first, preserving
// the original (sorted) order within each group.
func prioritize(files, priority []string) []string {
	if len(priority) == 0 {
		return files
	}
	var head, tail []string
	for _, f := range files {
		if matchesAny(f, priority) {
			head = append(head, f)
		} else {
			tail = append(tail, f)
		}
	}
	return append(head, tail...)
}

func matchesAny(file string, priority []string) bool {
	base := path.Base(file)
	for _, p := range priority {
		if p == "" {
			continue
		}
		if strings.Contains(file, p) || path.Base(p) == base {
			return true
		}
	}
	return false
}

var sourceExts = map[string]bool{
	".go": true, ".yaml": true, ".yml": true, ".sh": true,
	".py": true, ".json": true, ".tf": true, ".bicep": true,
}

func isSourceFile(name string) bool {
	if sourceExts[strings.ToLower(path.Ext(name))] {
		return true
	}
	switch path.Base(name) {
	case "Dockerfile", "Makefile":
		return true
	}
	return false
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func capStrings(s []string, n int) []string {
	sort.Strings(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
