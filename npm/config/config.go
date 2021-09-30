package npmconfig

const (
	defaultResyncPeriod  = 15
	defaultListeningPort = 10091

	// ConfigEnvPath is what's used by viper to load config path
	ConfigEnvPath = "NPM_CONFIG"
)

// DefaultConfig is the guaranteed configuration NPM can run in out of the box
var DefaultConfig = Config{
	ResyncPeriodInMinutes: defaultResyncPeriod,
	ListeningPort:         defaultListeningPort,
	ListeningAddress:      "0.0.0.0",
	Toggles: Toggles{
		EnablePrometheusMetrics: true,
		EnablePprof:             true,
		EnableHTTPDebugAPI:      true,
		EnableV2Controllers:     false,
	},
}

type Config struct {
	ResyncPeriodInMinutes int     `json:"ResyncPeriodInMinutes"`
	ListeningPort         int     `json:"ListeningPort"`
	ListeningAddress      string  `json:"ListeningAddress"`
	Toggles               Toggles `json:"Toggles"`
}

type Toggles struct {
	EnablePrometheusMetrics bool
	EnablePprof             bool
	EnableHTTPDebugAPI      bool
	EnableV2Controllers     bool
}
