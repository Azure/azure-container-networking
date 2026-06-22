package github

import (
	"context"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	goTitlePattern   = regexp.MustCompile(`(?i)go.*(upgrade|bump|version|1\.[0-9]+)`)
	goBranchPattern  = regexp.MustCompile(`(?i)go.*(bump|upgrade|version|minor)`)
	goLabelPattern   = regexp.MustCompile(`(?i)go-upgrade`)
	skipPRPattern    = regexp.MustCompile(`(?i)\bskip\s+#(\d+)`)
	pausePRPattern   = regexp.MustCompile(`(?i)\bpause\s+#(\d+)`)
	unpausePRPattern = regexp.MustCompile(`(?i)\bunpause\s+#(\d+)`)
	resumePRPattern  = regexp.MustCompile(`(?i)\bresume\s+#(\d+)`)
)

type execRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

type Client struct {
	run execRunner
}

type PR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

type CollectResult struct {
	PRs             []PR `json:"prs"`
	OpenCount       int  `json:"open_count"`
	DependabotCount int  `json:"dependabot_count"`
	GoUpgradeCount  int  `json:"go_upgrade_count"`
}

type WaitOptions struct {
	Repo         string
	Branch       string
	IssueNumber  int
	PollInterval time.Duration
	MaxWait      time.Duration // 0 means no internal timeout (relies on job-level timeout)
}

type WaitResult struct {
	MergedCount int    `json:"merged_count"`
	SkippedPRs  string `json:"skipped_prs"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghPR struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	HeadRefName string    `json:"headRefName"`
	Labels      []ghLabel `json:"labels"`
	State       string    `json:"state"`
}

type ghIssueComment struct {
	Body string `json:"body"`
}

type ghIssue struct {
	Comments []ghIssueComment `json:"comments"`
}

type prState struct {
	State string `json:"state"`
	Title string `json:"title"`
}

func NewClient() *Client {
	return &Client{run: runCommand}
}

func (c *Client) CollectPRs(ctx context.Context, repo, branch string) (CollectResult, error) {
	dependabotPRs, err := c.listDependabotPRs(ctx, repo, branch)
	if err != nil {
		return CollectResult{}, err
	}

	goUpgradePRs, err := c.listGoUpgradePRs(ctx, repo, branch)
	if err != nil {
		return CollectResult{}, err
	}

	merged := uniquePRs(dependabotPRs, goUpgradePRs)
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Number < merged[j].Number
	})

	return CollectResult{
		PRs:             merged,
		OpenCount:       len(merged),
		DependabotCount: len(dependabotPRs),
		GoUpgradeCount:  len(goUpgradePRs),
	}, nil
}

func (c *Client) WaitPRs(ctx context.Context, opts WaitOptions, progress io.Writer) (WaitResult, error) {
	if progress == nil {
		progress = io.Discard
	}

	// Apply internal timeout if configured
	if opts.MaxWait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.MaxWait)
		defer cancel()
	}

	skipped := map[int]struct{}{}
	seen := map[int]PR{}

	for {
		comments, err := c.issueComments(ctx, opts.Repo, opts.IssueNumber)
		if err != nil {
			return WaitResult{}, err
		}

		commandState := parseIssueCommands(comments)
		if commandState.barePauseCount > commandState.bareResumeCount {
			fmt.Fprintf(progress, "release paused on issue #%d; waiting for resume\n", opts.IssueNumber)
			if err := sleepContext(ctx, opts.PollInterval); err != nil {
				return WaitResult{}, err
			}
			continue
		}

		activePauses, blockedTitles, err := c.activePauseHolds(ctx, opts.Repo, commandState)
		if err != nil {
			return WaitResult{}, err
		}

		current, err := c.CollectPRs(ctx, opts.Repo, opts.Branch)
		if err != nil {
			return WaitResult{}, err
		}
		for _, pr := range current.PRs {
			seen[pr.Number] = pr
		}

		if commandState.skipAll {
			for _, pr := range current.PRs {
				skipped[pr.Number] = struct{}{}
			}
			if len(activePauses) == 0 {
				break
			}
			fmt.Fprintf(progress, "skip all received, but pause holds remain: %v\n", activePauses)
			if err := sleepContext(ctx, opts.PollInterval); err != nil {
				return WaitResult{}, err
			}
			continue
		}

		waiting := make([]PR, 0, len(current.PRs))
		for _, pr := range current.PRs {
			if _, ok := commandState.skipPRs[pr.Number]; ok {
				skipped[pr.Number] = struct{}{}
				continue
			}
			waiting = append(waiting, pr)
		}

		if len(waiting) == 0 && len(activePauses) == 0 {
			break
		}

		fmt.Fprintf(progress, "blocked on %d PR(s) and %d pause hold(s)\n", len(waiting), len(activePauses))
		for _, pr := range waiting {
			fmt.Fprintf(progress, "  waiting: #%d %s\n", pr.Number, pr.Title)
		}
		for _, prNumber := range activePauses {
			fmt.Fprintf(progress, "  pause hold: #%d %s\n", prNumber, blockedTitles[prNumber])
		}

		if err := sleepContext(ctx, opts.PollInterval); err != nil {
			return WaitResult{}, err
		}
	}

	mergedCount := 0
	for _, number := range sortedKeys(seen) {
		if _, ok := skipped[number]; ok {
			continue
		}

		state, err := c.prState(ctx, opts.Repo, number)
		if err != nil {
			return WaitResult{}, err
		}
		if state.State == "MERGED" {
			mergedCount++
		}
	}

	return WaitResult{
		MergedCount: mergedCount,
		SkippedPRs:  joinInts(sortedKeys(skipped)),
	}, nil
}

func (c *Client) listDependabotPRs(ctx context.Context, repo, branch string) ([]PR, error) {
	out, err := c.run(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--base", branch,
		"--author", "app/dependabot",
		"--state", "open",
		"--json", "number,title,url",
	)
	if err != nil {
		return nil, fmt.Errorf("listing dependabot PRs: %w", err)
	}

	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("decoding dependabot PRs: %w", err)
	}

	return prs, nil
}

func (c *Client) listGoUpgradePRs(ctx context.Context, repo, branch string) ([]PR, error) {
	out, err := c.run(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--base", branch,
		"--state", "open",
		"--json", "number,title,url,headRefName,labels",
	)
	if err != nil {
		return nil, fmt.Errorf("listing candidate Go upgrade PRs: %w", err)
	}

	var candidates []ghPR
	if err := json.Unmarshal(out, &candidates); err != nil {
		return nil, fmt.Errorf("decoding candidate Go upgrade PRs: %w", err)
	}

	matches := make([]PR, 0, len(candidates))
	for _, pr := range candidates {
		if !isGoUpgradePR(pr) {
			continue
		}
		matches = append(matches, PR{
			Number: pr.Number,
			Title:  pr.Title,
			URL:    pr.URL,
		})
	}

	return matches, nil
}

func (c *Client) issueComments(ctx context.Context, repo string, issueNumber int) ([]string, error) {
	out, err := c.run(ctx, "gh", "issue", "view",
		strconv.Itoa(issueNumber),
		"--repo", repo,
		"--json", "comments",
	)
	if err != nil {
		return nil, fmt.Errorf("getting issue comments: %w", err)
	}

	var issue ghIssue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("decoding issue comments: %w", err)
	}

	comments := make([]string, 0, len(issue.Comments))
	for _, comment := range issue.Comments {
		comments = append(comments, comment.Body)
	}

	return comments, nil
}

func (c *Client) activePauseHolds(ctx context.Context, repo string, state issueCommandState) ([]int, map[int]string, error) {
	active := make([]int, 0, len(state.pausePRs))
	blockedTitles := make(map[int]string, len(state.pausePRs))
	for number := range state.pausePRs {
		if _, ok := state.resumePRs[number]; ok {
			continue
		}

		prState, err := c.prState(ctx, repo, number)
		if err != nil {
			continue
		}
		if prState.State != "OPEN" {
			continue
		}

		active = append(active, number)
		blockedTitles[number] = prState.Title
	}

	sort.Ints(active)
	return active, blockedTitles, nil
}

func (c *Client) prState(ctx context.Context, repo string, number int) (prState, error) {
	out, err := c.run(ctx, "gh", "pr", "view",
		strconv.Itoa(number),
		"--repo", repo,
		"--json", "state,title",
	)
	if err != nil {
		return prState{}, fmt.Errorf("getting PR #%d state: %w", number, err)
	}

	var state prState
	if err := json.Unmarshal(out, &state); err != nil {
		return prState{}, fmt.Errorf("decoding PR #%d state: %w", number, err)
	}

	return state, nil
}

func isGoUpgradePR(pr ghPR) bool {
	if goTitlePattern.MatchString(pr.Title) || goBranchPattern.MatchString(pr.HeadRefName) {
		return true
	}

	return slices.ContainsFunc(pr.Labels, func(label ghLabel) bool {
		return goLabelPattern.MatchString(label.Name)
	})
}

func uniquePRs(groups ...[]PR) []PR {
	seen := map[int]PR{}
	for _, group := range groups {
		for _, pr := range group {
			if _, ok := seen[pr.Number]; ok {
				continue
			}
			seen[pr.Number] = pr
		}
	}

	merged := make([]PR, 0, len(seen))
	for _, pr := range seen {
		merged = append(merged, pr)
	}

	return merged
}

type issueCommandState struct {
	barePauseCount  int
	bareResumeCount int
	skipAll         bool
	skipPRs         map[int]struct{}
	pausePRs        map[int]struct{}
	resumePRs       map[int]struct{}
}

func parseIssueCommands(comments []string) issueCommandState {
	state := issueCommandState{
		skipPRs:   map[int]struct{}{},
		pausePRs:  map[int]struct{}{},
		resumePRs: map[int]struct{}{},
	}

	for _, comment := range comments {
		for _, line := range strings.Split(comment, "\n") {
			switch strings.ToLower(strings.TrimSpace(line)) {
			case "pause":
				state.barePauseCount++
			case "resume":
				state.bareResumeCount++
			case "skip all":
				state.skipAll = true
			}
		}

		for _, number := range extractNumbers(skipPRPattern, comment) {
			state.skipPRs[number] = struct{}{}
		}
		for _, number := range extractNumbers(pausePRPattern, comment) {
			state.pausePRs[number] = struct{}{}
		}
		for _, number := range extractNumbers(unpausePRPattern, comment) {
			state.resumePRs[number] = struct{}{}
		}
		for _, number := range extractNumbers(resumePRPattern, comment) {
			state.resumePRs[number] = struct{}{}
		}
	}

	return state
}

func extractNumbers(pattern *regexp.Regexp, text string) []int {
	matches := pattern.FindAllStringSubmatch(text, -1)
	numbers := make([]int, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		numbers = append(numbers, number)
	}

	return numbers
}

func sortedKeys[T any](m map[int]T) []int {
	keys := make([]int, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	return keys
}

func joinInts(numbers []int) string {
	parts := make([]string, 0, len(numbers))
	for _, number := range numbers {
		parts = append(parts, strconv.Itoa(number))
	}

	return strings.Join(parts, ",")
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}
