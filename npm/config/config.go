package npmconfig

var DefaultConfig = Config{
	ResyncPeriodInMinutes: 15,
	ListeningPort:         10091,
	ListeningAddress:      "0.0.0.0",
	Toggles: Toggles{
		EnablePrometheusMetrics: true,
		EnablePprof:             true,
		EnableHTTPDebugAPI:      true,
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
}
