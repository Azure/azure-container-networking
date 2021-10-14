//go:build integration
// +build integration

package goldpinger

import "fmt"

type ClusterStats CheckAllJSON

func (c ClusterStats) AllPodsHealthy() bool {
	if len(c.Responses) == 0 {
		return false
	}

	for _, podStatus := range c.Responses {
		if !podStatus.OK {
			return false
		}
	}
	return true
}

func (c ClusterStats) AllPingsHealthy() bool {
	if !c.AllPodsHealthy() {
		return false
	}
	for _, podStatus := range c.Responses {
		for _, pingResult := range podStatus.Response.PodResults {
			if !pingResult.OK {
				return false
			}
		}
	}
	return true
}

type stringSet map[string]struct{}

func (set stringSet) add(s string) {
	set[s] = struct{}{}
}

func (c ClusterStats) PrintStats() {
	podCount := len(c.Hosts)
	nodes := make(stringSet)
	healthyPods := make(stringSet)
	pingCount := 0
	healthyPingCount := 0

	for _, podStatus := range c.Responses {
		nodes.add(podStatus.HostIP)
		if podStatus.OK {
			healthyPods.add(podStatus.PodIP)
		}
		for _, pingStatus := range podStatus.Response.PodResults {
			pingCount++
			if pingStatus.OK {
				healthyPingCount++
			}
		}
	}

	format := "cluster stats - " +
		"nodes in use: %d, " +
		"pod count: %d, " +
		"pod health: %d/%d (%2.2f), " +
		"ping health percentage: %d/%d (%2.2f)\n"

	podHealthPct := (float64(len(healthyPods)) / float64(podCount)) * 100
	pingHealthPct := (float64(healthyPingCount) / float64(pingCount)) * 100
	fmt.Printf(format, len(nodes), podCount, len(healthyPods), podCount, podHealthPct, healthyPingCount, pingCount, pingHealthPct)
}
