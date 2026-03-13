package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/azure/aksmigrate/pkg/types"
)

// PrintAuditReport outputs the audit report in the specified format.
func PrintAuditReport(report *types.AuditReport, format string) error {
	switch format {
	case "json":
		return printJSON(report)
	case "table":
		return printAuditTable(report)
	default:
		return printAuditTable(report)
	}
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printAuditTable(report *types.AuditReport) error {
	fmt.Printf("\n")
	fmt.Printf("NPM-to-Cilium Migration Audit Report\n")
	fmt.Printf("=====================================\n")
	if report.ClusterName != "" {
		fmt.Printf("Cluster:   %s\n", report.ClusterName)
	}
	fmt.Printf("Timestamp: %s\n", report.Timestamp)
	fmt.Printf("Policies:  %d\n", report.TotalPolicies)
	fmt.Printf("\n")

	// Summary
	fmt.Printf("Summary\n")
	fmt.Printf("-------\n")
	fmt.Printf("  FAIL: %d\n", report.Summary.FailCount)
	fmt.Printf("  WARN: %d\n", report.Summary.WarnCount)
	fmt.Printf("  INFO: %d\n", report.Summary.InfoCount)
	fmt.Printf("\n")

	if len(report.Findings) == 0 {
		fmt.Printf("No issues found. Cluster is ready for migration.\n")
		return nil
	}

	// Group findings by severity
	for _, severity := range []types.Severity{types.SeverityFail, types.SeverityWarn, types.SeverityInfo} {
		findings := filterBySeverity(report.Findings, severity)
		if len(findings) == 0 {
			continue
		}

		icon := severityIcon(severity)
		fmt.Printf("%s %s Findings (%d)\n", icon, severity, len(findings))
		fmt.Printf("%s\n", strings.Repeat("-", 60))

		for _, f := range findings {
			location := f.Namespace + "/" + f.PolicyName
			if f.Namespace == "" {
				location = f.PolicyName
			}
			fmt.Printf("\n  [%s] %s\n", f.RuleID, location)
			fmt.Printf("  %s\n", f.Description)
			if len(f.AffectedFields) > 0 {
				fmt.Printf("  Fields: %s\n", strings.Join(f.AffectedFields, ", "))
			}
			fmt.Printf("  Fix: %s\n", f.Remediation)
		}
		fmt.Printf("\n")
	}

	// Final verdict
	if report.Summary.FailCount > 0 {
		fmt.Printf("RESULT: MIGRATION BLOCKED - %d critical issues must be resolved before migration.\n", report.Summary.FailCount)
	} else if report.Summary.WarnCount > 0 {
		fmt.Printf("RESULT: MIGRATION POSSIBLE WITH WARNINGS - %d warnings should be reviewed.\n", report.Summary.WarnCount)
	} else {
		fmt.Printf("RESULT: READY FOR MIGRATION\n")
	}
	fmt.Printf("\n")
	return nil
}

func filterBySeverity(findings []types.Finding, severity types.Severity) []types.Finding {
	var result []types.Finding
	for _, f := range findings {
		if f.Severity == severity {
			result = append(result, f)
		}
	}
	return result
}

func severityIcon(s types.Severity) string {
	switch s {
	case types.SeverityFail:
		return "[X]"
	case types.SeverityWarn:
		return "[!]"
	case types.SeverityInfo:
		return "[i]"
	default:
		return "[ ]"
	}
}

// PrintTranslationSummary outputs a summary of translation actions taken.
func PrintTranslationSummary(output *types.TranslationOutput) {
	fmt.Printf("\n")
	fmt.Printf("NPM-to-Cilium Policy Translation Summary\n")
	fmt.Printf("=========================================\n")
	fmt.Printf("Patched NetworkPolicies:      %d\n", len(output.PatchedPolicies))
	fmt.Printf("Generated CiliumNetworkPolicies: %d\n", len(output.CiliumPolicies))
	fmt.Printf("Named ports resolved:         %d\n", len(output.RemovedNamedPorts))
	fmt.Printf("\n")

	if len(output.PatchedPolicies) > 0 {
		fmt.Printf("Patched Policies:\n")
		for _, pp := range output.PatchedPolicies {
			fmt.Printf("  %s/%s:\n", pp.Original.Namespace, pp.Original.Name)
			for _, c := range pp.Changes {
				fmt.Printf("    - %s\n", c)
			}
		}
		fmt.Printf("\n")
	}

	if len(output.CiliumPolicies) > 0 {
		fmt.Printf("Generated CiliumNetworkPolicies:\n")
		for _, cp := range output.CiliumPolicies {
			fmt.Printf("  %s/%s\n", cp.Namespace, cp.Name)
			fmt.Printf("    Reason: %s\n", cp.Reason)
		}
		fmt.Printf("\n")
	}

	if len(output.RemovedNamedPorts) > 0 {
		fmt.Printf("Named Port Resolutions:\n")
		for _, m := range output.RemovedNamedPorts {
			fmt.Printf("  %s/%s: %s -> %d/%s\n", m.Namespace, m.PolicyName, m.PortName, m.PortNumber, m.Protocol)
		}
		fmt.Printf("\n")
	}
}
