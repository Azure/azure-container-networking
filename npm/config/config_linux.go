//go:build !windows
// +build !windows

package npmconfig

// GetConfigPath TODO: solidify config paths
func GetConfigPath() string {
	return "/etc/azure/azure-vnet/azure-npm.json"
}
