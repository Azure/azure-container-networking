package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	statevalidate "github.com/Azure/azure-container-networking/test/validate"
)

type options struct {
	baselinePath    string
	candidatePath   string
	expectedBackend statevalidate.StateBackend
}

type summaryStats struct {
	Checks      int `json:"checks"`
	LivePods    int `json:"livePods"`
	ExpectedIPs int `json:"expectedIPs"`
	ActualIPs   int `json:"actualIPs"`
}

type compareOutput struct {
	Baseline  summaryStats `json:"baseline"`
	Candidate summaryStats `json:"candidate"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	baseline, err := readSummary(opts.baselinePath, opts.expectedBackend)
	if err != nil {
		fmt.Fprintf(stderr, "reading baseline summary: %v\n", err)
		return 2
	}
	candidate, err := readSummary(opts.candidatePath, opts.expectedBackend)
	if err != nil {
		fmt.Fprintf(stderr, "reading candidate summary: %v\n", err)
		return 2
	}

	output := compareOutput{
		Baseline:  aggregate(baseline),
		Candidate: aggregate(candidate),
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		fmt.Fprintf(stderr, "encoding comparison output: %v\n", err)
		return 2
	}

	if err := statevalidate.CompareValidationSummaries(baseline, candidate, opts.expectedBackend); err != nil {
		fmt.Fprintf(stderr, "summary comparison failed: %v\n", err)
		return 1
	}
	return 0
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	flags := flag.NewFlagSet("summarydiff", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var opts options
	flags.StringVar(&opts.baselinePath, "baseline", "", "path to baseline validation summary JSON")
	flags.StringVar(&opts.candidatePath, "candidate", "", "path to candidate validation summary JSON")
	expectedBackend := flags.String("expected-backend", "", "required state backend")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	opts.expectedBackend = statevalidate.StateBackend(*expectedBackend)
	var missing []string
	if opts.baselinePath == "" {
		missing = append(missing, "-baseline")
	}
	if opts.candidatePath == "" {
		missing = append(missing, "-candidate")
	}
	if opts.expectedBackend == "" {
		missing = append(missing, "-expected-backend")
	}
	if len(missing) != 0 {
		return options{}, fmt.Errorf("required flags missing: %v", missing)
	}
	return opts, nil
}

func readSummary(path string, expectedBackend statevalidate.StateBackend) (statevalidate.ValidationSummary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return statevalidate.ValidationSummary{}, fmt.Errorf("reading %q: %w", path, err)
	}

	summary, err := statevalidate.DecodeValidationSummary(bytes.NewReader(raw), expectedBackend)
	if err != nil {
		return statevalidate.ValidationSummary{}, fmt.Errorf("decoding %q: %w", path, err)
	}
	return summary, nil
}

func aggregate(summary statevalidate.ValidationSummary) summaryStats {
	stats := summaryStats{Checks: len(summary.Checks)}
	for i := range summary.Checks {
		stats.LivePods += summary.Checks[i].LivePodCount
		stats.ExpectedIPs += len(summary.Checks[i].Expected)
		stats.ActualIPs += len(summary.Checks[i].Actual)
	}
	return stats
}
