package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/azure/aksmigrate/pkg/types"
)

// FleetDiscoverer discovers AKS clusters that use Azure NPM across subscriptions.
type FleetDiscoverer struct {
	subscriptionID string
}

// NewFleetDiscoverer creates a new FleetDiscoverer for the given subscription.
func NewFleetDiscoverer(subscriptionID string) *FleetDiscoverer {
	return &FleetDiscoverer{
		subscriptionID: subscriptionID,
	}
}

// azGraphResult represents the JSON structure returned by az graph query.
type azGraphResult struct {
	Data []azClusterRecord `json:"data"`
}

// azClusterRecord represents a single cluster record from Azure Resource Graph.
type azClusterRecord struct {
	Name              string `json:"name"`
	ResourceGroup     string `json:"resourceGroup"`
	SubscriptionID    string `json:"subscriptionId"`
	Location          string `json:"location"`
	KubernetesVersion string `json:"kubernetesVersion"`
	NetworkPlugin     string `json:"networkPlugin"`
	NetworkPolicy     string `json:"networkPolicy"`
	NetworkDataplane  string `json:"networkDataplane"`
	AgentPoolCount    int    `json:"agentPoolCount"`
	HasWindowsPools   bool   `json:"hasWindowsPools"`
}

// DiscoverClusters uses "az graph query" to find all NPM-enabled AKS clusters
// across subscriptions using the Azure Resource Graph.
func (d *FleetDiscoverer) DiscoverClusters(ctx context.Context) ([]types.ClusterInfo, error) {
	// Azure Resource Graph query to find all AKS clusters with NPM network policy.
	query := `
Resources
| where type =~ 'Microsoft.ContainerService/managedClusters'
| where properties.networkProfile.networkPolicy =~ 'azure'
| project
    name,
    resourceGroup,
    subscriptionId,
    location,
    kubernetesVersion = properties.kubernetesVersion,
    networkPlugin = properties.networkProfile.networkPlugin,
    networkPolicy = properties.networkProfile.networkPolicy,
    networkDataplane = properties.networkProfile.networkDataplane,
    agentPoolProfiles = properties.agentPoolProfiles
| extend agentPoolCount = array_length(agentPoolProfiles)
| extend hasWindowsPools = iff(agentPoolProfiles contains '"osType":"Windows"', true, false)
| project-away agentPoolProfiles
`

	args := []string{
		"graph", "query",
		"-q", strings.TrimSpace(query),
		"--output", "json",
	}

	// Scope to a specific subscription if provided.
	if d.subscriptionID != "" {
		args = append(args, "--subscriptions", d.subscriptionID)
	}

	cmd := exec.CommandContext(ctx, "az", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("az graph query failed: %s\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("running az graph query: %w", err)
	}

	var result azGraphResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing az graph query output: %w", err)
	}

	var clusters []types.ClusterInfo
	for _, rec := range result.Data {
		clusters = append(clusters, types.ClusterInfo{
			Name:              rec.Name,
			ResourceGroup:     rec.ResourceGroup,
			SubscriptionID:    rec.SubscriptionID,
			Location:          rec.Location,
			KubernetesVersion: rec.KubernetesVersion,
			NetworkPlugin:     rec.NetworkPlugin,
			NetworkPolicy:     rec.NetworkPolicy,
			NetworkDataplane:  rec.NetworkDataplane,
			NodeCount:         rec.AgentPoolCount,
			HasWindowsPools:   rec.HasWindowsPools,
		})
	}

	fmt.Printf("Discovered %d NPM-enabled AKS clusters\n", len(clusters))
	return clusters, nil
}

// PrioritizeMigration assigns risk levels and migration order to the discovered clusters.
// Clusters without Windows pools, on newer Kubernetes versions, and with fewer policies
// are prioritized for earlier migration.
func (d *FleetDiscoverer) PrioritizeMigration(clusters []types.ClusterInfo) *types.MigrationPlan {
	// Assign risk levels based on cluster characteristics.
	for i := range clusters {
		clusters[i].RiskLevel = assessRisk(clusters[i])
	}

	// Sort clusters by migration priority.
	sort.SliceStable(clusters, func(i, j int) bool {
		return migrationScore(clusters[i]) < migrationScore(clusters[j])
	})

	// Assign migration order.
	for i := range clusters {
		clusters[i].MigrationOrder = i + 1
	}

	// Build fleet summary.
	summary := types.FleetSummary{
		TotalClusters: len(clusters),
	}
	for _, c := range clusters {
		if c.HasWindowsPools {
			summary.BlockedByWindows++
		} else if c.RiskLevel == "high" {
			summary.NeedsRemediation++
		} else {
			summary.ReadyToMigrate++
		}
	}

	return &types.MigrationPlan{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Clusters:  clusters,
		Summary:   summary,
	}
}

// assessRisk determines the risk level for a cluster migration.
func assessRisk(c types.ClusterInfo) string {
	// Windows pools are a blocker; mark as high risk.
	if c.HasWindowsPools {
		return "high"
	}

	// Older Kubernetes versions may have compatibility issues.
	if strings.Compare(c.KubernetesVersion, "1.28") < 0 {
		return "high"
	}

	// High policy count increases migration risk.
	if c.PolicyCount > 50 {
		return "medium"
	}

	// Large clusters carry more risk.
	if c.NodeCount > 100 {
		return "medium"
	}

	return "low"
}

// migrationScore returns a numeric score for sorting; lower scores migrate first.
func migrationScore(c types.ClusterInfo) int {
	score := 0

	// Windows pools push clusters to the end.
	if c.HasWindowsPools {
		score += 10000
	}

	// Higher risk goes later.
	switch c.RiskLevel {
	case "high":
		score += 3000
	case "medium":
		score += 2000
	case "low":
		score += 1000
	}

	// More policies means more risk.
	score += c.PolicyCount * 10

	// More nodes means more risk.
	score += c.NodeCount

	// Newer K8s versions are preferred (lower score).
	// Invert the version comparison so newer versions get lower scores.
	if strings.Compare(c.KubernetesVersion, "1.30") >= 0 {
		score -= 500
	} else if strings.Compare(c.KubernetesVersion, "1.29") >= 0 {
		score -= 300
	} else if strings.Compare(c.KubernetesVersion, "1.28") >= 0 {
		score -= 100
	}

	return score
}
