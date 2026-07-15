package grounding

import (
	"context"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/tools/failure-agent/internal/model"
)

// fakeGit maps a git subcommand (by its first two args) to canned output.
type fakeGit struct {
	nameOnly string
	stat     string
	diffs    map[string]string // file -> diff
	log      string
	shows    map[string]string // "head:file" -> content
	calls    [][]string
}

func (f *fakeGit) runner() GitRunner {
	return func(_ context.Context, args ...string) (string, error) {
		f.calls = append(f.calls, args)
		switch {
		case len(args) >= 2 && args[0] == "diff" && args[1] == "--name-only":
			return f.nameOnly, nil
		case len(args) >= 2 && args[0] == "diff" && args[1] == "--stat":
			return f.stat, nil
		case len(args) >= 4 && args[0] == "diff" && args[2] == "--":
			return f.diffs[args[3]], nil
		case len(args) >= 1 && args[0] == "log":
			return f.log, nil
		case len(args) >= 2 && args[0] == "show":
			return f.shows[args[1]], nil
		}
		return "", nil
	}
}

func TestCollectGathersChangeUnderTest(t *testing.T) {
	fg := &fakeGit{
		nameOnly: "network/manager.go\ncns/restserver/util.go\n",
		stat:     " network/manager.go | 9 +++++\n cns/restserver/util.go | 3 +-",
		diffs: map[string]string{
			"network/manager.go":     "@@ -1,3 +1,4 @@\n+added a nil deref\n",
			"cns/restserver/util.go": "@@ -10,2 +10,2 @@\n-old\n+new\n",
		},
		log:   "abc123\x1ffix: remove required field\ndef456\x1frefactor manager",
		shows: map[string]string{"HEAD:network/manager.go": "package network\nfunc X() {}\n"},
	}

	cc := Collect(context.Background(), Options{
		BaseRef:        "origin/master",
		Priority:       []string{"network/manager.go"},
		SourceExcerpts: true,
	}, fg.runner())

	if len(cc.ChangedFiles) != 2 {
		t.Fatalf("changed files: got %v", cc.ChangedFiles)
	}
	if !strings.Contains(cc.Diff, "added a nil deref") {
		t.Errorf("diff missing manager change: %q", cc.Diff)
	}
	if len(cc.Commits) != 2 || cc.Commits[0].Subject != "fix: remove required field" {
		t.Errorf("commits: got %+v", cc.Commits)
	}
	if _, ok := cc.SourceExcerpts["network/manager.go"]; !ok {
		t.Errorf("expected source excerpt for prioritized go file, got %v", cc.SourceExcerpts)
	}
	if cc.IsEmpty() {
		t.Error("expected non-empty code context")
	}
}

func TestCollectNoBaseRefIsNoop(t *testing.T) {
	fg := &fakeGit{nameOnly: "a.go"}
	cc := Collect(context.Background(), Options{BaseRef: ""}, fg.runner())
	if !cc.IsEmpty() || len(fg.calls) != 0 {
		t.Errorf("expected no-op with empty base ref, calls=%d", len(fg.calls))
	}
}

func TestCollectPrioritizesImplicatedFileInDiff(t *testing.T) {
	fg := &fakeGit{
		nameOnly: "aaa/first.go\nzzz/culprit.go\n",
		diffs: map[string]string{
			"aaa/first.go":   "@@ first @@\n",
			"zzz/culprit.go": "@@ culprit @@\n",
		},
	}
	cc := Collect(context.Background(), Options{
		BaseRef:  "origin/master",
		Priority: []string{"zzz/culprit.go"},
	}, fg.runner())

	if !strings.HasPrefix(cc.Diff, "@@ culprit @@") {
		t.Errorf("expected implicated file diff first, got %q", cc.Diff)
	}
}

func TestCandidatePathsFromLines(t *testing.T) {
	lines := []string{
		"panic at network/manager.go:123 nil pointer",
		"error in cns/restserver/util.go",
		"no path here",
		"duplicate network/manager.go again",
	}
	got := CandidatePathsFromLines(lines)
	want := map[string]bool{"network/manager.go": true, "cns/restserver/util.go": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected candidate %q", g)
		}
	}
}

func TestDetectVersionsEnvWinsOverEvidence(t *testing.T) {
	ev := model.Evidence{
		TopErrorLines: []string{"Server Version: v1.30.2"},
		Excerpts:      map[string]string{"live/pods": "image: acr.io/azure-cns:v1.5.0-evidence"},
	}
	getenv := func(k string) string {
		if k == "CILIUM_VERSION" {
			return "1.16.1"
		}
		return ""
	}
	v := DetectVersions(getenv, ev)
	if v["kubernetes"] != "1.30.2" {
		t.Errorf("kubernetes: got %q", v["kubernetes"])
	}
	if v["azure-cns"] != "v1.5.0-evidence" {
		t.Errorf("azure-cns: got %q", v["azure-cns"])
	}
	if v["cilium"] != "1.16.1" {
		t.Errorf("cilium env should win: got %q", v["cilium"])
	}
}

func TestBaseAndHeadRef(t *testing.T) {
	pr := map[string]string{
		"SYSTEM_PULLREQUEST_TARGETBRANCH":   "refs/heads/release/v1.6",
		"SYSTEM_PULLREQUEST_SOURCECOMMITID": "deadbeef",
	}
	if got := BaseRef(func(k string) string { return pr[k] }); got != "origin/release/v1.6" {
		t.Errorf("BaseRef: got %q", got)
	}
	if got := HeadRef(func(k string) string { return pr[k] }); got != "deadbeef" {
		t.Errorf("HeadRef: got %q", got)
	}
	empty := func(string) string { return "" }
	if got := BaseRef(empty); got != "origin/master" {
		t.Errorf("BaseRef default: got %q", got)
	}
	if got := HeadRef(empty); got != "HEAD" {
		t.Errorf("HeadRef default: got %q", got)
	}
}
