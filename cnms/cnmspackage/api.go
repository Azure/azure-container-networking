package cnms

import "github.com/Azure/azure-container-networking/telemetry"

type NetworkMonitor struct {
	AddRulesToBeValidated    map[string]int
	DeleteRulesToBeValidated map[string]int
	CNIReport                *telemetry.CNIReport
}
