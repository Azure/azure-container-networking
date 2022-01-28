package npmconfig

const (
	defaultResyncPeriod  = 15
	defaultListeningPort = 10091
	defaultGrpcPort      = 10092
	// ConfigEnvPath is what's used by viper to load config path
	ConfigEnvPath = "NPM_CONFIG"
)

// DefaultConfig is the guaranteed configuration NPM can run in out of the box
var DefaultConfig = Config{
	ResyncPeriodInMinutes: defaultResyncPeriod,

	ListeningPort:    defaultListeningPort,
	ListeningAddress: "0.0.0.0",

	Transport: GrpcServerConfig{
		Address: "0.0.0.0",
		Port:    defaultGrpcPort,
	},

	Toggles: Toggles{
		EnablePrometheusMetrics: true,
		EnablePprof:             true,
		EnableHTTPDebugAPI:      true,
		EnableV2NPM:             false,
		PlaceAzureChainFirst:    false,
		ApplyIPSetsOnNeed:       false,
	},
}

type GrpcServerConfig struct {
	// Address is the address on which the gRPC server will listen
	Address string `json:"Address,omitempty"`
	// Port is the port on which the gRPC server will listen
	Port int `json:"Port,omitempty"`
}

type Config struct {
	ResyncPeriodInMinutes int `json:"ResyncPeriodInMinutes,omitempty"`

	ListeningPort    int    `json:"ListeningPort,omitempty"`
	ListeningAddress string `json:"ListeningAddress,omitempty"`

	Transport GrpcServerConfig `json:"Transport,omitempty"`

	Toggles Toggles `json:"Toggles,omitempty"`
}

type Toggles struct {
	EnablePrometheusMetrics bool
	EnablePprof             bool
	EnableHTTPDebugAPI      bool
	EnableV2NPM             bool
	PlaceAzureChainFirst    bool
	ApplyIPSetsOnNeed       bool
}

type Flags struct {
	KubeConfigPath string `json:"KubeConfigPath"`
}
