//go:build windows
// +build windows

package npmconfig

//TODO: solidify config paths
func GetConfigPath() string {
	return "c:\\k\\azure-npm\\azure-npm.json"
}
