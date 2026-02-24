package connectivity

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/azure/aksmigrate/pkg/types"
)

// PrintDiffReport prints a table showing regressions and new allows from a connectivity diff.
func PrintDiffReport(diff *types.ConnectivityDiff) {
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("CONNECTIVITY DIFF REPORT")
	fmt.Printf("Pre-snapshot:  %s (%s)\n", diff.PreSnapshot.Phase, diff.PreSnapshot.Timestamp)
	fmt.Printf("Post-snapshot: %s (%s)\n", diff.PostSnapshot.Phase, diff.PostSnapshot.Timestamp)
	fmt.Println(strings.Repeat("=", 100))

	// Summary line.
	fmt.Printf("\nUnchanged: %d | Regressions: %d | New Allows: %d\n\n",
		diff.Unchanged, len(diff.Regressions), len(diff.NewAllows))

	// Print regressions table.
	if len(diff.Regressions) > 0 {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Println("REGRESSIONS (was working, now broken)")
		fmt.Println(strings.Repeat("-", 100))
		printResultTable(diff.Regressions)
	} else {
		fmt.Println("No regressions detected.")
	}

	fmt.Println()

	// Print new allows table.
	if len(diff.NewAllows) > 0 {
		fmt.Println(strings.Repeat("-", 100))
		fmt.Println("NEW ALLOWS (was blocked, now working)")
		fmt.Println(strings.Repeat("-", 100))
		printResultTable(diff.NewAllows)
	} else {
		fmt.Println("No new allows detected.")
	}

	fmt.Println(strings.Repeat("=", 100))
}

// printResultTable prints a formatted table of connectivity results.
func printResultTable(results []types.ConnectivityResult) {
	header := fmt.Sprintf("  %-20s %-30s %-30s %-6s %-10s %s",
		"SOURCE NS", "SOURCE POD", "TARGET", "PORT", "TYPE", "ERROR")
	fmt.Println(header)
	fmt.Println("  " + strings.Repeat("-", 98))

	for _, r := range results {
		errMsg := r.Error
		if len(errMsg) > 40 {
			errMsg = errMsg[:37] + "..."
		}
		fmt.Printf("  %-20s %-30s %-30s %-6d %-10s %s\n",
			truncate(r.Probe.SourceNamespace, 20),
			truncate(r.Probe.SourcePod, 30),
			truncate(r.Probe.TargetAddress, 30),
			r.Probe.TargetPort,
			r.Probe.ProbeType,
			errMsg,
		)
	}
}

// truncate shortens a string to maxLen, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// SaveSnapshot saves a ConnectivitySnapshot as JSON to the given file path.
func SaveSnapshot(snapshot *types.ConnectivitySnapshot, path string) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing snapshot to %s: %w", path, err)
	}

	fmt.Printf("Snapshot saved to %s (%d results)\n", path, len(snapshot.Results))
	return nil
}

// LoadSnapshot loads a ConnectivitySnapshot from a JSON file at the given path.
func LoadSnapshot(path string) (*types.ConnectivitySnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot from %s: %w", path, err)
	}

	var snapshot types.ConnectivitySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshaling snapshot: %w", err)
	}

	return &snapshot, nil
}
