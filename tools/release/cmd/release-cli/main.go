package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	githubcli "github.com/Azure/azure-container-networking/tools/release/internal/github"
	"github.com/Azure/azure-container-networking/tools/release/internal/notify"
	"github.com/Azure/azure-container-networking/tools/release/internal/pipeline"
	"github.com/Azure/azure-container-networking/tools/release/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "release-cli",
		Short: "Utilities for the scheduled release workflow",
	}

	rootCmd.AddCommand(
		newNotifyCommand(),
		newNotifyReplyCommand(),
		newCollectPRsCommand(),
		newNextVersionCommand(),
		newDetectBinariesCommand(),
		newWaitPipelineCommand(),
		newWaitPRsCommand(),
	)

	return rootCmd
}

func newNotifyCommand() *cobra.Command {
	var (
		channelID string
		title     string
		status    string
		summary   string
		stage     string
		teamID    string
		runID     string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Create or update a Teams adaptive card",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := notify.NotificationRequest{
				Source:    notify.SourceScheduledRelease,
				RunID:     runID,
				TeamID:    teamID,
				ChannelID: channelID,
				Title:     title,
				Status:    normalizeNotifyStatus(status),
				Summary:   summary,
				Stage:     stage,
			}
			if err := validateNotificationRequest(req); err != nil {
				return err
			}

			if dryRun {
				return writeJSON(map[string]any{
					"dry_run": true,
					"request": req,
				})
			}

			client := notify.NewClientFromEnv()
			body, err := client.Notify(cmd.Context(), req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: notify failed: %v\n", err)
				return writeJSONBody(body)
			}

			return writeJSONBody(body)
		},
	}

	cmd.Flags().StringVar(&channelID, "channel-id", "", "Teams channel ID")
	cmd.Flags().StringVar(&title, "title", "", "Card title")
	cmd.Flags().StringVar(&status, "status", "", "Card status")
	cmd.Flags().StringVar(&summary, "summary", "", "Card summary")
	cmd.Flags().StringVar(&stage, "stage", "", "Release stage")
	cmd.Flags().StringVar(&teamID, "team-id", notify.DefaultTeamID(), "Teams team ID")
	cmd.Flags().StringVar(&runID, "run-id", notify.DefaultRunID(), "Release run ID")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the request without sending it")
	mustMarkFlagRequired(cmd, "channel-id", "title", "status", "summary")

	return cmd
}

func newNotifyReplyCommand() *cobra.Command {
	var (
		text     string
		tag      string
		severity string
		runID    string
	)

	cmd := &cobra.Command{
		Use:   "notify-reply",
		Short: "Post a reply under an existing release card",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := notify.ReplyRequest{
				Source:   notify.SourceScheduledRelease,
				RunID:    runID,
				Text:     text,
				Tag:      tag,
				Severity: severity,
			}
			if err := validateReplyRequest(req); err != nil {
				return err
			}

			client := notify.NewClientFromEnv()
			body, err := client.Reply(cmd.Context(), req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: notify-reply failed: %v\n", err)
				return writeJSONBody(body)
			}

			return writeJSONBody(body)
		},
	}

	cmd.Flags().StringVar(&text, "text", "", "Reply body")
	cmd.Flags().StringVar(&tag, "tag", "", "Optional reply tag")
	cmd.Flags().StringVar(&severity, "severity", "", "Optional reply severity")
	cmd.Flags().StringVar(&runID, "run-id", notify.DefaultRunID(), "Release run ID")
	mustMarkFlagRequired(cmd, "text")

	return cmd
}

func newCollectPRsCommand() *cobra.Command {
	var (
		repo   string
		branch string
	)

	cmd := &cobra.Command{
		Use:   "collect-prs",
		Short: "Collect pending dependabot and Go upgrade PRs",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(repo) == "" {
				return errors.New("repo is required")
			}
			if strings.TrimSpace(branch) == "" {
				return errors.New("branch is required")
			}

			client := githubcli.NewClient()
			result, err := client.CollectPRs(cmd.Context(), repo, branch)
			if err != nil {
				return err
			}

			return writeJSON(result)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "GitHub repository in OWNER/REPO form")
	cmd.Flags().StringVar(&branch, "branch", "", "Base branch")
	mustMarkFlagRequired(cmd, "branch")

	return cmd
}

func newNextVersionCommand() *cobra.Command {
	var branch string

	cmd := &cobra.Command{
		Use:   "next-version",
		Short: "Determine the next patch version for a branch",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := version.NewService()
			result, err := svc.NextVersion(cmd.Context(), branch)
			if err != nil {
				return err
			}

			return writeJSON(result)
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "", "Branch or ref to inspect")
	mustMarkFlagRequired(cmd, "branch")

	return cmd
}

func newDetectBinariesCommand() *cobra.Command {
	var branch string

	cmd := &cobra.Command{
		Use:   "detect-binaries",
		Short: "Detect binaries that need a release tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := version.NewService()
			result, err := svc.DetectBinaryChanges(cmd.Context(), branch)
			if err != nil {
				return err
			}

			return writeJSON(result)
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "", "Branch or ref to inspect")
	mustMarkFlagRequired(cmd, "branch")

	return cmd
}

func newWaitPipelineCommand() *cobra.Command {
	var (
		org          string
		project      string
		definitionID string
		token        string
		tag          string
		timeout      time.Duration
		name         string
		dryRun       bool
	)

	cmd := &cobra.Command{
		Use:   "wait-pipeline",
		Short: "Find and wait for an ADO pipeline run triggered by a tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := pipeline.WaitOptions{
				Org:          org,
				Project:      project,
				DefinitionID: definitionID,
				Token:        token,
				Tag:          tag,
				Timeout:      timeout,
				Name:         name,
			}
			if err := validateWaitPipelineOptions(opts); err != nil {
				return err
			}

			if dryRun {
				return writeJSON(map[string]any{
					"dry_run": true,
					"request": opts,
				})
			}

			client := pipeline.NewClient()
			result, err := client.WaitForRun(cmd.Context(), opts, os.Stderr)
			if err != nil {
				return err
			}

			return writeJSON(result)
		},
	}

	cmd.Flags().StringVar(&org, "org", "", "ADO organization")
	cmd.Flags().StringVar(&project, "project", "", "ADO project")
	cmd.Flags().StringVar(&definitionID, "definition-id", "", "ADO pipeline definition ID")
	cmd.Flags().StringVar(&token, "token", "", "Bearer token (from OIDC)")
	cmd.Flags().StringVar(&tag, "tag", "", "Git tag to look for")
	cmd.Flags().DurationVar(&timeout, "timeout", 90*time.Minute, "Overall timeout")
	cmd.Flags().StringVar(&name, "name", "", "Friendly pipeline name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the request without polling")
	mustMarkFlagRequired(cmd, "org", "project", "definition-id", "token", "tag", "name")

	return cmd
}

func newWaitPRsCommand() *cobra.Command {
	var (
		repo         string
		branch       string
		issueNumber  int
		pollInterval time.Duration
		maxWait      time.Duration
	)

	cmd := &cobra.Command{
		Use:   "wait-prs",
		Short: "Wait for PRs to merge or be skipped",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(repo) == "" {
				return errors.New("repo is required")
			}
			if strings.TrimSpace(branch) == "" {
				return errors.New("branch is required")
			}
			if issueNumber <= 0 {
				return errors.New("issue-number must be greater than zero")
			}
			if pollInterval <= 0 {
				return errors.New("poll-interval must be greater than zero")
			}

			client := githubcli.NewClient()
			result, err := client.WaitPRs(cmd.Context(), githubcli.WaitOptions{
				Repo:         repo,
				Branch:       branch,
				IssueNumber:  issueNumber,
				PollInterval: pollInterval,
				MaxWait:      maxWait,
			}, os.Stderr)
			if err != nil {
				return err
			}

			return writeJSON(result)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "GitHub repository in OWNER/REPO form")
	cmd.Flags().StringVar(&branch, "branch", "", "Base branch")
	cmd.Flags().IntVar(&issueNumber, "issue-number", 0, "Tracking issue number")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 5*time.Minute, "Polling interval")
	cmd.Flags().DurationVar(&maxWait, "max-wait", 4*time.Hour, "Maximum wait time (0 = no limit)")
	mustMarkFlagRequired(cmd, "branch", "issue-number")

	return cmd
}

func validateNotificationRequest(req notify.NotificationRequest) error {
	switch {
	case strings.TrimSpace(req.RunID) == "":
		return errors.New("run-id is required (or set GITHUB_RUN_ID)")
	case strings.TrimSpace(req.TeamID) == "":
		return errors.New("team-id is required (or set TEAMS_TEAM_ID)")
	case strings.TrimSpace(req.ChannelID) == "":
		return errors.New("channel-id is required")
	case strings.TrimSpace(req.Title) == "":
		return errors.New("title is required")
	case strings.TrimSpace(req.Summary) == "":
		return errors.New("summary is required")
	}

	if _, ok := allowedNotifyStatuses[req.Status]; !ok {
		return fmt.Errorf("unsupported status %q", req.Status)
	}

	return nil
}

func validateReplyRequest(req notify.ReplyRequest) error {
	switch {
	case strings.TrimSpace(req.RunID) == "":
		return errors.New("run-id is required (or set GITHUB_RUN_ID)")
	case strings.TrimSpace(req.Text) == "":
		return errors.New("text is required")
	default:
		return nil
	}
}

func validateWaitPipelineOptions(opts pipeline.WaitOptions) error {
	switch {
	case strings.TrimSpace(opts.Org) == "":
		return errors.New("org is required")
	case strings.TrimSpace(opts.Project) == "":
		return errors.New("project is required")
	case strings.TrimSpace(opts.DefinitionID) == "":
		return errors.New("definition-id is required")
	case strings.TrimSpace(opts.Token) == "":
		return errors.New("token is required")
	case strings.TrimSpace(opts.Tag) == "":
		return errors.New("tag is required")
	case strings.TrimSpace(opts.Name) == "":
		return errors.New("name is required")
	case opts.Timeout <= 0:
		return errors.New("timeout must be greater than zero")
	default:
		return nil
	}
}

func normalizeNotifyStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "in_progress":
		return "running"
	case "success":
		return "succeeded"
	case "failure":
		return "failed"
	case "cancelled":
		return "canceled"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func mustMarkFlagRequired(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if err := cmd.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}
}

func writeJSON(v any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func writeJSONBody(body []byte) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return writeJSON(map[string]any{})
	}

	if json.Valid(trimmed) {
		var out any
		if err := json.Unmarshal(trimmed, &out); err != nil {
			return err
		}
		return writeJSON(out)
	}

	return writeJSON(map[string]string{
		"response": string(trimmed),
	})
}

var allowedNotifyStatuses = map[string]struct{}{
	"queued":    {},
	"running":   {},
	"succeeded": {},
	"failed":    {},
	"canceled":  {},
}
