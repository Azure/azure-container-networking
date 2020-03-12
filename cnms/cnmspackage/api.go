package cnms

type NetworkMonitor struct {
	AddRulesToBeValidated    map[string]int
	DeleteRulesToBeValidated map[string]int
}
