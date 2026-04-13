package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type validationSummary struct {
	Checks []validationCheckEntry `json:"checks,omitempty"`
}

type validationCheckEntry struct {
	ExpectedCount  int      `json:"expectedCount"`
	ActualCount    int      `json:"actualCount"`
	MissingIPs     []string `json:"missingIPs,omitempty"`
	UnexpectedIPs  []string `json:"unexpectedIPs,omitempty"`
	DuplicateIPs   []string `json:"duplicateIPs,omitempty"`
	ValidationPass bool     `json:"validationPass"`
}

type summaryStats struct {
	TotalChecks    int `json:"totalChecks"`
	FailedChecks   int `json:"failedChecks"`
	MissingIPs     int `json:"missingIPs"`
	UnexpectedIPs  int `json:"unexpectedIPs"`
	DuplicateIPs   int `json:"duplicateIPs"`
	ExpectedIPsSum int `json:"expectedIPsSum"`
	ActualIPsSum   int `json:"actualIPsSum"`
}

type compareOutput struct {
	Baseline  summaryStats `json:"baseline"`
	Candidate summaryStats `json:"candidate"`
}

func main() {
	baselinePath := flag.String("baseline", "", "Path to baseline validation summary JSON")
	candidatePath := flag.String("candidate", "", "Path to candidate validation summary JSON")
	flag.Parse()

	if *baselinePath == "" || *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "usage: summarydiff -baseline <path> -candidate <path>")
		os.Exit(2)
	}

	baseline, err := readSummary(*baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read baseline summary: %v\n", err)
		os.Exit(2)
	}
	candidate, err := readSummary(*candidatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read candidate summary: %v\n", err)
		os.Exit(2)
	}

	baselineStats := aggregate(baseline)
	candidateStats := aggregate(candidate)

	output := compareOutput{Baseline: baselineStats, Candidate: candidateStats}
	raw, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal output: %v\n", err)
		os.Exit(2)
	}
	fmt.Println(string(raw))

	if candidateStats.TotalChecks != baselineStats.TotalChecks {
		fmt.Fprintf(
			os.Stderr,
			"summarydiff failed: total checks mismatch baseline=%d candidate=%d\n",
			baselineStats.TotalChecks,
			candidateStats.TotalChecks,
		)
		os.Exit(1)
	}

	var regressions []string
	if candidateStats.FailedChecks > baselineStats.FailedChecks {
		regressions = append(regressions, fmt.Sprintf("FailedChecks: %d > %d", candidateStats.FailedChecks, baselineStats.FailedChecks))
	}
	if candidateStats.MissingIPs > baselineStats.MissingIPs {
		regressions = append(regressions, fmt.Sprintf("MissingIPs: %d > %d", candidateStats.MissingIPs, baselineStats.MissingIPs))
	}
	if candidateStats.UnexpectedIPs > baselineStats.UnexpectedIPs {
		regressions = append(regressions, fmt.Sprintf("UnexpectedIPs: %d > %d", candidateStats.UnexpectedIPs, baselineStats.UnexpectedIPs))
	}
	if candidateStats.DuplicateIPs > baselineStats.DuplicateIPs {
		regressions = append(regressions, fmt.Sprintf("DuplicateIPs: %d > %d", candidateStats.DuplicateIPs, baselineStats.DuplicateIPs))
	}
	if len(regressions) > 0 {
		fmt.Fprintf(os.Stderr, "summarydiff failed: candidate regressed: %s\n", regressions)
		os.Exit(1)
	}
}

func readSummary(path string) (validationSummary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return validationSummary{}, err
	}

	var s validationSummary
	if err := json.Unmarshal(raw, &s); err != nil {
		return validationSummary{}, err
	}
	return s, nil
}

func aggregate(s validationSummary) summaryStats {
	stats := summaryStats{}
	for _, check := range s.Checks {
		stats.TotalChecks++
		stats.ExpectedIPsSum += check.ExpectedCount
		stats.ActualIPsSum += check.ActualCount
		stats.MissingIPs += len(check.MissingIPs)
		stats.UnexpectedIPs += len(check.UnexpectedIPs)
		stats.DuplicateIPs += len(check.DuplicateIPs)
		if !check.ValidationPass {
			stats.FailedChecks++
		}
	}
	return stats
}
