// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	cnscli "github.com/Azure/azure-container-networking/cns/cmd/cli"
	"github.com/Azure/azure-container-networking/cns/cniconflist"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/deviceplugin"
	"github.com/Azure/azure-container-networking/cns/endpointmanager"
	"github.com/Azure/azure-container-networking/cns/fsnotify"
	"github.com/Azure/azure-container-networking/cns/grpc"
	"github.com/Azure/azure-container-networking/cns/healthserver"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/imds"
	"github.com/Azure/azure-container-networking/cns/ipampool"
	"github.com/Azure/azure-container-networking/cns/ipampool/metrics"
	ipampoolv2 "github.com/Azure/azure-container-networking/cns/ipampool/v2"
	cssctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/clustersubnetstate"
	mtpncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/multitenantpodnetworkconfig"
	nncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/nodenetworkconfig"
	podctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/pod"
	"github.com/Azure/azure-container-networking/cns/logger"
	loggerv2 "github.com/Azure/azure-container-networking/cns/logger/v2"
	"github.com/Azure/azure-container-networking/cns/metric"
	"github.com/Azure/azure-container-networking/cns/middlewares"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/multitenantoperator"
	"github.com/Azure/azure-container-networking/cns/nodesetup"
	"github.com/Azure/azure-container-networking/cns/restserver"
	restserverv2 "github.com/Azure/azure-container-networking/cns/restserver/v2"
	cnipodprovider "github.com/Azure/azure-container-networking/cns/stateprovider/cni"
	cnspodprovider "github.com/Azure/azure-container-networking/cns/stateprovider/cns"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate"
	cssv1alpha1 "github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/multitenancy"
	mtv1alpha1 "github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	acnfs "github.com/Azure/azure-container-networking/internal/fs"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/nmagent"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	localtls "github.com/Azure/azure-container-networking/server/tls"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/avast/retry-go/v4"
	"github.com/go-logr/zapr"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	// Service name.
	name                              = "azure-cns"
	pluginName                        = "azure-vnet"
	endpointStoreName                 = "azure-endpoints"
	endpointStoreLocationLinux        = "/var/run/azure-cns/"
	endpointStoreLocationWindows      = "/k/azurecns/"
	defaultCNINetworkConfigFileName   = "10-azure.conflist"
	dncApiVersion                     = "?api-version=2018-03-01"
	poolIPAMRefreshRateInMilliseconds = 1000

	// 720 * acn.FiveSeconds sec sleeps = 1Hr
	maxRetryNodeRegister = 720
	initCNSInitalDelay   = 10 * time.Second

	// envVarEnableCNIConflistGeneration enables cni conflist generation if set (value doesn't matter)
	envVarEnableCNIConflistGeneration = "CNS_ENABLE_CNI_CONFLIST_GENERATION"

	cnsReqTimeout                    = 15 * time.Second
	defaultLocalServerIP             = "localhost"
	defaultLocalServerPort           = "10090"
	defaultDevicePluginRetryInterval = 2 * time.Second
	defaultNodeInfoCRDPollInterval   = 5 * time.Second
	defaultDevicePluginMaxRetryCount = 5
	initialVnetNICCount              = 0
	initialIBNICCount                = 0
)

type cniConflistScenario string

const (
	scenarioV4Overlay        cniConflistScenario = "v4overlay"
	scenarioDualStackOverlay cniConflistScenario = "dualStackOverlay"
	scenarioOverlay          cniConflistScenario = "overlay"
	scenarioCilium           cniConflistScenario = "cilium"
	scenarioSWIFT            cniConflistScenario = "swift"
)

var (
	rootCtx   context.Context
	rootErrCh chan error
)

// Version is populated by make during build.
var version string

// endpointStorePath is used to create the path for EdnpointState file.
var endpointStorePath string

// Command line arguments for CNS.
var args = acn.ArgumentList{
	{
		Name:         acn.OptEnvironment,
		Shorthand:    acn.OptEnvironmentAlias,
		Description:  "Set the operating environment",
		Type:         "string",
		DefaultValue: acn.OptEnvironmentAzure,
		ValueMap: map[string]interface{}{
			acn.OptEnvironmentAzure:    0,
			acn.OptEnvironmentMAS:      0,
			acn.OptEnvironmentFileIpam: 0,
		},
	},
	{
		Name:         acn.OptAPIServerURL,
		Shorthand:    acn.OptAPIServerURLAlias,
		Description:  "Set the API server URL",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptLogLevel,
		Shorthand:    acn.OptLogLevelAlias,
		Description:  "Set the logging level",
		Type:         "int",
		DefaultValue: acn.OptLogLevelInfo,
		ValueMap: map[string]interface{}{
			acn.OptLogLevelInfo:  log.LevelInfo,
			acn.OptLogLevelDebug: log.LevelDebug,
		},
	},
	{
		Name:         acn.OptLogTarget,
		Shorthand:    acn.OptLogTargetAlias,
		Description:  "Set the logging target",
		Type:         "int",
		DefaultValue: acn.OptLogTargetFile,
		ValueMap: map[string]interface{}{
			acn.OptLogTargetSyslog: log.TargetSyslog,
			acn.OptLogTargetStderr: log.TargetStderr,
			acn.OptLogTargetFile:   log.TargetLogfile,
			acn.OptLogStdout:       log.TargetStdout,
			acn.OptLogMultiWrite:   log.TargetStdOutAndLogFile,
		},
	},
	{
		Name:         acn.OptLogLocation,
		Shorthand:    acn.OptLogLocationAlias,
		Description:  "Set the directory location where logs will be saved",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptIpamQueryUrl,
		Shorthand:    acn.OptIpamQueryUrlAlias,
		Description:  "Set the IPAM query URL",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptIpamQueryInterval,
		Shorthand:    acn.OptIpamQueryIntervalAlias,
		Description:  "Set the IPAM plugin query interval",
		Type:         "int",
		DefaultValue: "",
	},
	{
		Name:         acn.OptCnsURL,
		Shorthand:    acn.OptCnsURLAlias,
		Description:  "Set the URL for CNS to listen on",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptCnsPort,
		Shorthand:    acn.OptCnsPortAlias,
		Description:  "Set the URL port for CNS to listen on",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptVersion,
		Shorthand:    acn.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptStoreFileLocation,
		Shorthand:    acn.OptStoreFileLocationAlias,
		Description:  "Set store file absolute path",
		Type:         "string",
		DefaultValue: platform.CNMRuntimePath,
	},
	{
		Name:         acn.OptNetPluginPath,
		Shorthand:    acn.OptNetPluginPathAlias,
		Description:  "Set network plugin binary absolute path to parent (of azure-vnet and azure-vnet-ipam)",
		Type:         "string",
		DefaultValue: platform.K8SCNIRuntimePath,
	},
	{
		Name:         acn.OptNetPluginConfigFile,
		Shorthand:    acn.OptNetPluginConfigFileAlias,
		Description:  "Set network plugin configuration file absolute path",
		Type:         "string",
		DefaultValue: platform.K8SNetConfigPath + string(os.PathSeparator) + defaultCNINetworkConfigFileName,
	},
	{
		Name:         acn.OptCreateDefaultExtNetworkType,
		Shorthand:    acn.OptCreateDefaultExtNetworkTypeAlias,
		Description:  "Create default external network for windows platform with the specified type (l2bridge)",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptTelemetry,
		Shorthand:    acn.OptTelemetryAlias,
		Description:  "Set to false to disable telemetry. This is deprecated in favor of cns_config.json",
		Type:         "bool",
		DefaultValue: true,
	},
	{
		Name:         acn.OptHttpConnectionTimeout,
		Shorthand:    acn.OptHttpConnectionTimeoutAlias,
		Description:  "Set HTTP connection timeout in seconds to be used by http client in CNS",
		Type:         "int",
		DefaultValue: "5",
	},
	{
		Name:         acn.OptHttpResponseHeaderTimeout,
		Shorthand:    acn.OptHttpResponseHeaderTimeoutAlias,
		Description:  "Set HTTP response header timeout in seconds to be used by http client in CNS",
		Type:         "int",
		DefaultValue: "120",
	},
	{
		Name:         acn.OptPrivateEndpoint,
		Shorthand:    acn.OptPrivateEndpointAlias,
		Description:  "Set private endpoint",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptInfrastructureNetworkID,
		Shorthand:    acn.OptInfrastructureNetworkIDAlias,
		Description:  "Set infrastructure network ID",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptNodeID,
		Shorthand:    acn.OptNodeIDAlias,
		Description:  "Set node name/ID",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptManaged,
		Shorthand:    acn.OptManagedAlias,
		Description:  "Set to true to enable managed mode. This is deprecated in favor of cns_config.json",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptDebugCmd,
		Shorthand:    acn.OptDebugCmdAlias,
		Description:  "Debug flag to retrieve IPconfigs, available values: assigned, available, all",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptDebugArg,
		Shorthand:    acn.OptDebugArgAlias,
		Description:  "Argument flag to be paired with the 'debugcmd' flag.",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptCNSConfigPath,
		Shorthand:    acn.OptCNSConfigPathAlias,
		Description:  "Path to cns config file",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptTelemetryService,
		Shorthand:    acn.OptTelemetryServiceAlias,
		Description:  "Flag to start telemetry service to receive telemetry events from CNI. Default, disabled.",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptCNIConflistFilepath,
		Shorthand:    acn.OptCNIConflistFilepathAlias,
		Description:  "Filepath to write CNI conflist when CNI conflist generation is enabled",
		Type:         "string",
		DefaultValue: "",
	},
	{
		Name:         acn.OptCNIConflistScenario,
		Shorthand:    acn.OptCNIConflistScenarioAlias,
		Description:  "Scenario to generate CNI conflist for",
		Type:         "string",
		DefaultValue: "",
	},
}

// init() is executed before main() whenever this package is imported
// to do pre-run setup of things like exit signal handling and building
// the root context.
func init() {
	var cancel context.CancelFunc
	rootCtx, cancel = context.WithCancel(context.Background())

	rootErrCh = make(chan error, 1)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Fill EndpointStatePath based on the platform
	if endpointStorePath = os.Getenv("CNSStoreFilePath"); endpointStorePath == "" {
		if runtime.GOOS == "windows" {
			endpointStorePath = endpointStoreLocationWindows
		} else {
			endpointStorePath = endpointStoreLocationLinux
		}
	}
	go func() {
		// Wait until receiving a signal.
		select {
		case sig := <-sigCh:
			logger.Errorf("caught exit signal %v, exiting", sig)
		case err := <-rootErrCh:
			logger.Errorf("unhandled error %v, exiting", err)
		}
		cancel()
	}()
}

// Prints description and version information.
func printVersion() {
	fmt.Printf("Azure Container Network Service\n")
	fmt.Printf("Version %v\n", version)
}

// NodeInterrogator is functionality necessary to read information about nodes.
// It is intended to be strictly read-only.
type NodeInterrogator interface {
	SupportedAPIs(context.Context) ([]string, error)
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RegisterNode - Tries to register node with DNC when CNS is started in managed DNC mode
func registerNode(ctx context.Context, httpClient httpDoer, httpRestService cns.HTTPService, dncEP, infraVnet, nodeID string, ni NodeInterrogator) error {
	logger.Printf("[Azure CNS] Registering node %s with Infrastructure Network: %s PrivateEndpoint: %s", nodeID, infraVnet, dncEP)

	var (
		numCPU              = runtime.NumCPU()
		url                 = fmt.Sprintf(acn.RegisterNodeURLFmt, dncEP, infraVnet, nodeID, dncApiVersion)
		nodeRegisterRequest cns.NodeRegisterRequest
	)

	nodeRegisterRequest.NumCores = numCPU
	supportedApis, retErr := ni.SupportedAPIs(context.TODO())

	if retErr != nil {
		return errors.Wrap(retErr, fmt.Sprintf("[Azure CNS] Failed to retrieve SupportedApis from NMagent of node %s with Infrastructure Network: %s PrivateEndpoint: %s",
			nodeID, infraVnet, dncEP))
	}

	// To avoid any null-pointer de-referencing errors.
	if supportedApis == nil {
		supportedApis = []string{}
	}

	nodeRegisterRequest.NmAgentSupportedApis = supportedApis

	// CNS tries to register Node for maximum of an hour.
	err := retry.Do(func() error {
		return errors.Wrap(sendRegisterNodeRequest(ctx, httpClient, httpRestService, nodeRegisterRequest, url), "failed to sendRegisterNodeRequest")
	}, retry.Delay(acn.FiveSeconds), retry.Attempts(maxRetryNodeRegister), retry.DelayType(retry.FixedDelay))

	return errors.Wrap(err, fmt.Sprintf("[Azure CNS] Failed to register node %s after maximum reties for an hour with Infrastructure Network: %s PrivateEndpoint: %s",
		nodeID, infraVnet, dncEP))
}

// sendRegisterNodeRequest func helps in registering the node until there is an error.
func sendRegisterNodeRequest(ctx context.Context, httpClient httpDoer, httpRestService cns.HTTPService, nodeRegisterRequest cns.NodeRegisterRequest, registerURL string) error {
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(nodeRegisterRequest)
	if err != nil {
		logger.Errorf("Failed to register node while encoding json failed with non-retryable err %v", err)
		return errors.Wrap(retry.Unrecoverable(err), "failed to sendRegisterNodeRequest")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, &body)
	if err != nil {
		return errors.Wrap(err, "failed to build request")
	}

	request.Header.Set("Content-Type", "application/json")
	response, err := httpClient.Do(request)
	if err != nil {
		return errors.Wrap(err, "http request failed")
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("[Azure CNS] Failed to register node, DNC replied with http status code %s", strconv.Itoa(response.StatusCode))
		logger.Errorf(err.Error())
		return errors.Wrap(err, "failed to sendRegisterNodeRequest")
	}

	var req cns.SetOrchestratorTypeRequest
	err = json.NewDecoder(response.Body).Decode(&req)
	if err != nil {
		logger.Errorf("decoding Node Register response json failed with err %v", err)
		return errors.Wrap(err, "failed to sendRegisterNodeRequest")
	}
	httpRestService.SetNodeOrchestrator(&req)

	logger.Printf("[Azure CNS] Node Registered")
	return nil
}

func startTelemetryService(ctx context.Context) {
	var config aitelemetry.AIConfig

	tb := telemetry.NewTelemetryBuffer(nil)
	err := tb.CreateAITelemetryHandle(config, false, false, false)
	if err != nil {
		logger.Errorf("AI telemetry handle creation failed: %v", err)
		return
	}

	tbtemp := telemetry.NewTelemetryBuffer(nil)
	//nolint:errcheck // best effort to cleanup leaked pipe/socket before start
	tbtemp.Cleanup(telemetry.FdName)

	err = tb.StartServer()
	logger.Printf("Telemetry service for CNI started")
	if err != nil {
		logger.Errorf("Telemetry service failed to start: %v", err)
		return
	}
	tb.PushData(ctx)
}

// Main is the entry point for CNS.
func main() {
	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)

	cniPath := acn.GetArg(acn.OptNetPluginPath).(string)
	cniConfigFile := acn.GetArg(acn.OptNetPluginConfigFile).(string)
	cnsURL := acn.GetArg(acn.OptCnsURL).(string)
	cnsPort := acn.GetArg(acn.OptCnsPort).(string)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)

	vers := acn.GetArg(acn.OptVersion).(bool)
	createDefaultExtNetworkType := acn.GetArg(acn.OptCreateDefaultExtNetworkType).(string)
	telemetryEnabled := acn.GetArg(acn.OptTelemetry).(bool)
	httpConnectionTimeout := acn.GetArg(acn.OptHttpConnectionTimeout).(int)
	httpResponseHeaderTimeout := acn.GetArg(acn.OptHttpResponseHeaderTimeout).(int)
	storeFileLocation := acn.GetArg(acn.OptStoreFileLocation).(string)
	privateEndpoint := acn.GetArg(acn.OptPrivateEndpoint).(string)
	infravnet := acn.GetArg(acn.OptInfrastructureNetworkID).(string)
	nodeID := acn.GetArg(acn.OptNodeID).(string)
	clientDebugCmd := acn.GetArg(acn.OptDebugCmd).(string)
	clientDebugArg := acn.GetArg(acn.OptDebugArg).(string)
	cmdLineConfigPath := acn.GetArg(acn.OptCNSConfigPath).(string)
	telemetryDaemonEnabled := acn.GetArg(acn.OptTelemetryService).(bool)
	cniConflistFilepathArg := acn.GetArg(acn.OptCNIConflistFilepath).(string)
	cniConflistScenarioArg := acn.GetArg(acn.OptCNIConflistScenario).(string)

	if vers {
		printVersion()
		os.Exit(0)
	}

	// Initialize CNS.
	var (
		err                error
		config             common.ServiceConfig
		endpointStateStore store.KeyValueStore
	)

	config.Version = version
	config.Name = name
	// Create a channel to receive unhandled errors from CNS.
	config.ErrChan = rootErrCh

	// Create logging provider.
	logger.InitLogger(name, logLevel, logTarget, logDirectory)

	if clientDebugCmd != "" {
		err := cnscli.HandleCNSClientCommands(rootCtx, clientDebugCmd, clientDebugArg)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if !telemetryEnabled {
		logger.Errorf("[Azure CNS] Cannot disable telemetry via cmdline. Update cns_config.json to disable telemetry.")
	}

	cnsconfig, err := configuration.ReadConfig(cmdLineConfigPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			logger.Warnf("config file does not exist, using default")
			cnsconfig = &configuration.CNSConfig{}
		} else {
			logger.Errorf("fatal: failed to read cns config: %v", err)
			os.Exit(1)
		}
	}
	configuration.SetCNSConfigDefaults(cnsconfig)

	disableTelemetry := cnsconfig.TelemetrySettings.DisableAll
	if !disableTelemetry {
		ts := cnsconfig.TelemetrySettings
		aiConfig := aitelemetry.AIConfig{
			AppName:                      name,
			AppVersion:                   version,
			BatchSize:                    ts.TelemetryBatchSizeBytes,
			BatchInterval:                ts.TelemetryBatchIntervalInSecs,
			RefreshTimeout:               ts.RefreshIntervalInSecs,
			DisableMetadataRefreshThread: ts.DisableMetadataRefreshThread,
			DebugMode:                    ts.DebugMode,
		}

		if aiKey := cnsconfig.TelemetrySettings.AppInsightsInstrumentationKey; aiKey != "" {
			logger.InitAIWithIKey(aiConfig, aiKey, ts.DisableTrace, ts.DisableMetric, ts.DisableEvent)
		} else {
			logger.InitAI(aiConfig, ts.DisableTrace, ts.DisableMetric, ts.DisableEvent)
		}

		if cnsconfig.TelemetrySettings.ConfigSnapshotIntervalInMins > 0 {
			go metric.SendCNSConfigSnapshot(rootCtx, cnsconfig)
		}
	}
	logger.Printf("[Azure CNS] Using config: %+v", cnsconfig)

	_, envEnableConflistGeneration := os.LookupEnv(envVarEnableCNIConflistGeneration)
	var conflistGenerator restserver.CNIConflistGenerator
	if cnsconfig.EnableCNIConflistGeneration || envEnableConflistGeneration {
		conflistFilepath := cnsconfig.CNIConflistFilepath
		if cniConflistFilepathArg != "" {
			// allow the filepath to get overidden by command line arg
			conflistFilepath = cniConflistFilepathArg
		}
		writer, newWriterErr := acnfs.NewAtomicWriter(conflistFilepath)
		if newWriterErr != nil {
			logger.Errorf("unable to create atomic writer to generate cni conflist: %v", newWriterErr)
			os.Exit(1)
		}

		// allow the scenario to get overridden by command line arg
		scenarioString := cnsconfig.CNIConflistScenario
		if cniConflistScenarioArg != "" {
			scenarioString = cniConflistScenarioArg
		}

		switch scenario := cniConflistScenario(scenarioString); scenario {
		case scenarioV4Overlay:
			conflistGenerator = &cniconflist.V4OverlayGenerator{Writer: writer}
		case scenarioDualStackOverlay:
			conflistGenerator = &cniconflist.DualStackOverlayGenerator{Writer: writer}
		case scenarioOverlay:
			conflistGenerator = &cniconflist.OverlayGenerator{Writer: writer}
		case scenarioCilium:
			conflistGenerator = &cniconflist.CiliumGenerator{Writer: writer}
		case scenarioSWIFT:
			conflistGenerator = &cniconflist.SWIFTGenerator{Writer: writer}
		default:
			logger.Errorf("unable to generate cni conflist for unknown scenario: %s", scenario)
			os.Exit(1)
		}
	}

	// Get host metadata and attach it to logger(v2) appinsights config.
	// If this errors, we will not have metadata in the AI logs. Should we exit?
	metadata, _ := acn.GetHostMetadata(aitelemetry.MetadataFile)
	aifields := loggerv2.MetadataToFields(metadata)
	if cnsconfig.Logger.AppInsights != nil {
		cnsconfig.Logger.AppInsights.Fields = append(cnsconfig.Logger.AppInsights.Fields, aifields...)
	}

	// build the zap logger
	z, c, err := loggerv2.New(&cnsconfig.Logger)
	defer c()
	if err != nil {
		fmt.Printf("failed to create logger: %v", err)
		os.Exit(1)
	}
	host, _ := os.Hostname()
	z = z.With(zap.String("hostname", host), zap.String("version", version), zap.String("kubernetes_apiserver", os.Getenv("KUBERNETES_SERVICE_HOST")))
	config.Logger = z.With(zap.String("module", "cns service"))
	// Set the v2 logger to the global logger if v2 logger enabled.
	if cnsconfig.EnableLoggerV2 {
		logger.Printf("hotswapping logger v2") //nolint:staticcheck // ignore new deprecation
		logger.Log = loggerv2.AsV1(z, c)
	}

	// start the healthz/readyz/metrics server
	readyCh := make(chan any)
	readyChecker := healthz.CheckHandler{
		Checker: healthz.Checker(func(*http.Request) error {
			select {
			default:
				return errors.New("not ready")
			case <-readyCh:
			}
			return nil
		}),
	}

	healthzHandler, err := healthserver.NewHealthzHandlerWithChecks(&healthserver.Config{PingAPIServer: cnsconfig.EnableAPIServerHealthPing})
	if err != nil {
		logger.Errorf("unable to initialize a healthz handler: %v", err)
		return
	}
	go healthserver.Start(z, cnsconfig.MetricsBindAddress, healthzHandler, readyChecker)

	nmaConfig, err := nmagent.NewConfig(cnsconfig.WireserverIP)
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to produce NMAgent config from the supplied wireserver ip: %v", err)
		return
	}

	nmaClient, err := nmagent.NewClient(nmaConfig)
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to start nmagent client due to error: %v", err)
		return
	}

	// copy ChannelMode from cnsconfig to HTTPRemoteRestService config
	config.ChannelMode = cnsconfig.ChannelMode
	if cnsconfig.ChannelMode == cns.Managed {
		privateEndpoint = cnsconfig.ManagedSettings.PrivateEndpoint
		infravnet = cnsconfig.ManagedSettings.InfrastructureNetworkID
		nodeID = cnsconfig.ManagedSettings.NodeID
	}
	if isManaged, ok := acn.GetArg(acn.OptManaged).(bool); ok && isManaged {
		config.ChannelMode = cns.Managed
	}

	homeAzMonitor := restserver.NewHomeAzMonitor(nmaClient, time.Duration(cnsconfig.AZRSettings.PopulateHomeAzCacheRetryIntervalSecs)*time.Second)
	// homeAz monitor is only required when there is a direct channel between DNC and CNS.
	// This will prevent the monitor from unnecessarily calling NMA APIs for other scenarios such as AKS-swift, swiftv2
	if cnsconfig.ChannelMode == cns.Direct {
		homeAzMonitor.Start()
		defer homeAzMonitor.Stop()
	}

	if telemetryDaemonEnabled {
		logger.Printf("CNI Telemetry is enabled")
		go startTelemetryService(rootCtx)
	}

	// Log platform information.
	logger.Printf("Running on %v", platform.GetOSInfo())

	err = platform.CreateDirectory(storeFileLocation)
	if err != nil {
		logger.Errorf("Failed to create File Store directory %s, due to Error:%v", storeFileLocation, err.Error())
		return
	}

	lockclient, err := processlock.NewFileLock(platform.CNILockPath + name + store.LockExtension)
	if err != nil {
		logger.Printf("Error initializing file lock:%v", err)
		return
	}

	// Create the key value store.
	storeFileName := storeFileLocation + name + ".json"
	config.Store, err = store.NewJsonFileStore(storeFileName, lockclient, nil)
	if err != nil {
		logger.Errorf("Failed to create store file: %s, due to error %v\n", storeFileName, err)
		return
	}

	// Initialize endpoint state store if cns is managing endpoint state.
	if cnsconfig.ManageEndpointState {
		logger.Printf("[Azure CNS] Configured to manage endpoints state")
		endpointStoreLock, err := processlock.NewFileLock(platform.CNILockPath + endpointStoreName + store.LockExtension) // nolint
		if err != nil {
			logger.Printf("Error initializing endpoint state file lock:%v", err)
			return
		}
		defer endpointStoreLock.Unlock() // nolint

		err = platform.CreateDirectory(endpointStorePath)
		if err != nil {
			logger.Errorf("Failed to create File Store directory %s, due to Error:%v", storeFileLocation, err.Error())
			return
		}
		// Create the key value store.
		storeFileName := endpointStorePath + endpointStoreName + ".json"
		logger.Printf("EndpointStoreState path is %s", storeFileName)
		endpointStateStore, err = store.NewJsonFileStore(storeFileName, endpointStoreLock, nil)
		if err != nil {
			logger.Errorf("Failed to create endpoint state store file: %s, due to error %v\n", storeFileName, err)
			return
		}
	}

	wsProxy := wireserver.Proxy{
		Host:       cnsconfig.WireserverIP,
		HTTPClient: &http.Client{},
	}

	wsclient := &wireserver.Client{
		HostPort:   cnsconfig.WireserverIP,
		HTTPClient: &http.Client{},
		Logger:     logger.Log,
	}

	imdsClient := imds.NewClient()
	httpRemoteRestService, err := restserver.NewHTTPRestService(&config, wsclient, &wsProxy, &restserver.IPtablesProvider{}, nmaClient,
		endpointStateStore, conflistGenerator, homeAzMonitor, imdsClient)
	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	// Set CNS options.
	httpRemoteRestService.SetOption(acn.OptCnsURL, cnsURL)
	httpRemoteRestService.SetOption(acn.OptCnsPort, cnsPort)
	httpRemoteRestService.SetOption(acn.OptNetPluginPath, cniPath)
	httpRemoteRestService.SetOption(acn.OptNetPluginConfigFile, cniConfigFile)
	httpRemoteRestService.SetOption(acn.OptCreateDefaultExtNetworkType, createDefaultExtNetworkType)
	httpRemoteRestService.SetOption(acn.OptHttpConnectionTimeout, httpConnectionTimeout)
	httpRemoteRestService.SetOption(acn.OptHttpResponseHeaderTimeout, httpResponseHeaderTimeout)
	httpRemoteRestService.SetOption(acn.OptProgramSNATIPTables, cnsconfig.ProgramSNATIPTables)
	httpRemoteRestService.SetOption(acn.OptManageEndpointState, cnsconfig.ManageEndpointState)

	// Create default ext network if commandline option is set
	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.CreateDefaultExtNetwork(createDefaultExtNetworkType); err == nil {
			logger.Printf("[Azure CNS] Successfully created default ext network")
		} else {
			logger.Printf("[Azure CNS] Failed to create default ext network due to error: %v", err)
			return
		}
	}

	logger.Printf("[Azure CNS] Initialize HTTPRemoteRestService")
	if httpRemoteRestService != nil {
		if cnsconfig.UseHTTPS {
			config.TLSSettings = localtls.TlsSettings{
				TLSSubjectName:                     cnsconfig.TLSSubjectName,
				TLSCertificatePath:                 cnsconfig.TLSCertificatePath,
				TLSPort:                            cnsconfig.TLSPort,
				KeyVaultURL:                        cnsconfig.KeyVaultSettings.URL,
				KeyVaultCertificateName:            cnsconfig.KeyVaultSettings.CertificateName,
				MSIResourceID:                      cnsconfig.MSISettings.ResourceID,
				KeyVaultCertificateRefreshInterval: time.Duration(cnsconfig.KeyVaultSettings.RefreshIntervalInHrs) * time.Hour,
				UseMTLS:                            cnsconfig.UseMTLS,
				MinTLSVersion:                      cnsconfig.MinTLSVersion,
				MtlsClientCertSubjectName:          cnsconfig.MtlsClientCertSubjectName,
			}
		}

		err = httpRemoteRestService.Init(&config)
		if err != nil {
			logger.Errorf("Failed to init HTTPService, err:%v.\n", err)
			return
		}
	}

	// Setting the remote ARP MAC address to 12-34-56-78-9a-bc on windows for external traffic if HNS is enabled
	err = platform.SetSdnRemoteArpMacAddress(rootCtx)
	if err != nil {
		logger.Errorf("Failed to set remote ARP MAC address: %v", err)
		return
	}

	if cnsconfig.RouteWireserverViaDefaultInterface {
		if prepErr := nodesetup.New(cnsconfig, z).Run(); prepErr != nil {
			z.Error("failed to setup node", zap.Error(prepErr))
			return
		}
	}

	// We are only setting the PriorityVLANTag in 'cns.Direct' mode, because it neatly maps today, to 'isUsingMultitenancy'
	// In the future, we would want to have a better CNS flag, to explicitly say, this CNS is using multitenancy
	if cnsconfig.ChannelMode == cns.Direct {
		// Set Mellanox adapter's PriorityVLANTag value to 3 if adapter exists
		// reg key value for PriorityVLANTag = 3  --> Packet priority and VLAN enabled
		// for more details goto https://docs.nvidia.com/networking/display/winof2v230/Configuring+the+Driver+Registry+Keys#ConfiguringtheDriverRegistryKeys-GeneralRegistryKeysGeneralRegistryKeys
		if platform.HasMellanoxAdapter() {
			go platform.MonitorAndSetMellanoxRegKeyPriorityVLANTag(rootCtx, cnsconfig.MellanoxMonitorIntervalSecs)
		}

		// if swiftv2 scenario is enabled, we need to initialize the ServiceFabric(standalone) swiftv2 middleware to process IPConfigsRequests
		// RequestIPConfigs() will be invoked only for swiftv2 or swiftv1 k8s scenarios. For swiftv1 direct mode different api will be invoked.
		// So initializing this middleware always under direct mode should not affect any other scenarios
		swiftV2Middleware := &middlewares.StandaloneSWIFTv2Middleware{}
		httpRemoteRestService.AttachIPConfigsHandlerMiddleware(swiftV2Middleware)
	}

	// Initialze state in if CNS is running in CRD mode
	// State must be initialized before we start HTTPRestService
	if config.ChannelMode == cns.CRD {
		// Add APIServer FQDN to Log metadata
		logger.Log.SetAPIServer(os.Getenv("KUBERNETES_SERVICE_HOST"))

		// set logger in ctrlruntime
		ctrllog.SetLogger(zapr.NewLogger(z))

		// Check the CNI statefile mount, and if the file is empty
		// stub an empty JSON object
		if err := cnipodprovider.WriteObjectToCNIStatefile(); err != nil { //nolint:govet //shadow okay
			logger.Errorf("Failed to write empty object to CNI state: %v", err)
			return
		}

		// By default reinitialize state from the CNI.
		// Set the PodInfoVersion by initialization type, so that the
		// PodInfo maps use the correct key schema
		cns.GlobalPodInfoScheme = cns.InterfaceIDPodInfoScheme
		// If cns manageendpointstate is true, then cns maintains its own state and reconciles from it.
		// in this case, cns maintains state with containerid as key and so in-memory cache can lookup
		// and update based on container id.
		if cnsconfig.ManageEndpointState {
			cns.GlobalPodInfoScheme = cns.InfraIDPodInfoScheme
		}

		logger.Printf("Set GlobalPodInfoScheme %v (InitializeFromCNI=%t)", cns.GlobalPodInfoScheme, cnsconfig.InitializeFromCNI)

		err = InitializeCRDState(rootCtx, z, httpRemoteRestService, cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start CRD Controller, err:%v.\n", err)
			return
		}

		if cnsconfig.EnableSwiftV2 {
			// No-op for linux, mapping is set for windows in aks swiftv2 scenario
			logger.Printf("Fetching backend nics for the node")
			if err = httpRemoteRestService.SavePnpIDMacaddressMapping(rootCtx); err != nil {
				logger.Errorf("Failed to fetch PnpIDMacaddress mapping: %v", err)
			}

			// No-op for linux, setting primary macaddress if VF is enabled on the nics for aks swiftv2 windows
			logger.Printf("Setting VF for accelnet nics if feature is enabled (only on windows VM & swiftV2 scenario)")
			if err = httpRemoteRestService.SetVFForAccelnetNICs(); err != nil {
				logger.Errorf("Failed to set VF for accelnet NICs: %v", err)
			}
		}
	}

	// AzureHost channelmode indicates Nodesubnet. IPs are to be fetched from NMagent.
	if config.ChannelMode == cns.AzureHost {
		if !cnsconfig.ManageEndpointState {
			logger.Errorf("ManageEndpointState must be set to true for AzureHost mode")
			return
		}

		// If cns manageendpointstate is true, then cns maintains its own state and reconciles from it.
		// in this case, cns maintains state with containerid as key and so in-memory cache can lookup
		// and update based on container id.
		cns.GlobalPodInfoScheme = cns.InfraIDPodInfoScheme

		var podInfoByIPProvider cns.PodInfoByIPProvider
		podInfoByIPProvider, err = getPodInfoByIPProvider(rootCtx, cnsconfig, httpRemoteRestService, nil, "")
		if err != nil {
			logger.Errorf("[Azure CNS] Failed to get PodInfoByIPProvider: %v", err)
			return
		}

		err = httpRemoteRestService.InitializeNodeSubnet(rootCtx, podInfoByIPProvider)
		if err != nil {
			logger.Errorf("[Azure CNS] Failed to initialize node subnet: %v", err)
			return
		}
	}

	// Initialize multi-tenant controller if the CNS is running in MultiTenantCRD mode.
	// It must be started before we start HTTPRemoteRestService.
	if config.ChannelMode == cns.MultiTenantCRD {
		err = InitializeMultiTenantController(rootCtx, httpRemoteRestService, *cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start multiTenantController, err:%v.\n", err)
			return
		}
	}

	if cnsconfig.EnableSwiftV2 && cnsconfig.EnableK8sDevicePlugin {
		// Create device plugin manager instance
		pluginManager := deviceplugin.NewPluginManager(z)
		pluginManager.AddPlugin(mtv1alpha1.DeviceTypeVnetNIC, initialVnetNICCount)
		pluginManager.AddPlugin(mtv1alpha1.DeviceTypeInfiniBandNIC, initialIBNICCount)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start device plugin manager in a separate goroutine
		go func() {
			retryCount := 0
			ticker := time.NewTicker(defaultDevicePluginRetryInterval)
			// Ensure the ticker is stopped on exit
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					z.Info("Context canceled, stopping plugin manager")
					return
				case <-ticker.C:
					if pluginErr := pluginManager.Run(ctx); pluginErr != nil {
						z.Error("plugin manager exited with error", zap.Error(pluginErr))
						retryCount++
						// Implementing a basic circuit breaker
						if retryCount >= defaultDevicePluginMaxRetryCount {
							z.Error("Max retries reached, stopping plugin manager")
							return
						}
					} else {
						return
					}
				}
			}
		}()

		// go routine to poll node info crd and update device counts
		go func() {
			if pollErr := pollNodeInfoCRDAndUpdatePlugin(ctx, z, pluginManager); pollErr != nil {
				z.Error("Error in pollNodeInfoCRDAndUpdatePlugin", zap.Error(pollErr))
			}
		}()
	}

	// Conditionally initialize and start the gRPC server
	if cnsconfig.GRPCSettings.Enable {
		// Define gRPC server settings
		settings := grpc.ServerSettings{
			IPAddress: cnsconfig.GRPCSettings.IPAddress,
			Port:      cnsconfig.GRPCSettings.Port,
		}

		// Initialize CNS service
		cnsService := &grpc.CNS{Logger: z}

		// Create a new gRPC server
		server, grpcErr := grpc.NewServer(settings, cnsService, z)
		if grpcErr != nil {
			logger.Errorf("[Listener] Could not initialize gRPC server: %v", grpcErr)
			return
		}

		// Start the gRPC server
		go func() {
			if grpcErr := server.Start(); grpcErr != nil {
				logger.Errorf("[Listener] Could not start gRPC server: %v", grpcErr)
				return
			}
		}()
	}

	// if user provides cns url by -c option, then only start HTTP remote server using this url

	logger.Printf("[Azure CNS] Start HTTP Remote server")
	if httpRemoteRestService != nil {
		if cnsconfig.EnablePprof {
			httpRemoteRestService.RegisterPProfEndpoints()
		}

		err = httpRemoteRestService.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
			return
		}

	}

	// if user does not provide cns url by -c option, then start http local server
	// TODO: we will deprecated -c option in next phase and start local server in any case
	if config.Server.EnableLocalServer {
		logger.Printf("[Azure CNS] Start HTTP local server")

		var localServerURL string
		if config.Server.Port != "" {
			localServerURL = fmt.Sprintf("%s:%s", defaultLocalServerIP, config.Server.Port)
		} else {
			localServerURL = fmt.Sprintf("%s:%s", defaultLocalServerIP, defaultLocalServerPort)
		}

		httpLocalRestService := restserverv2.New(httpRemoteRestService)
		if httpLocalRestService != nil {
			go func() {
				err = httpLocalRestService.Start(rootCtx, localServerURL)
				if err != nil {
					logger.Errorf("Failed to start local echo server, err:%v.\n", err)
					return
				}
			}()
		}
	}

	if cnsconfig.EnableAsyncPodDelete {
		// Start fs watcher here
		z.Info("AsyncPodDelete is enabled")
		logger.Printf("AsyncPodDelete is enabled")
		cnsclient, err := cnsclient.New("", cnsReqTimeout) // nolint
		if err != nil {
			z.Error("failed to create cnsclient", zap.Error(err))
		}
		go func() {
			_ = retry.Do(func() error {
				z.Info("starting fsnotify watcher to process missed Pod deletes")
				logger.Printf("starting fsnotify watcher to process missed Pod deletes")
				var endpointCleanup fsnotify.ReleaseIPsClient = cnsclient
				// using endpointmanager implmentation for stateless CNI sceanrio to remove HNS endpoint alongside the IPs
				if cnsconfig.IsStalessCNIWindows() {
					endpointCleanup = endpointmanager.WithPlatformReleaseIPsManager(cnsclient)
				}
				w, err := fsnotify.New(endpointCleanup, cnsconfig.AsyncPodDeletePath, z)
				if err != nil {
					z.Error("failed to create fsnotify watcher", zap.Error(err))
					return errors.Wrap(err, "failed to create fsnotify watcher, will retry")
				}
				if err := w.Start(rootCtx); err != nil {
					z.Error("failed to start fsnotify watcher, will retry", zap.Error(err))
					return errors.Wrap(err, "failed to start fsnotify watcher, will retry")
				}
				return nil
			}, retry.DelayType(retry.BackOffDelay), retry.UntilSucceeded(), retry.Context(rootCtx)) // infinite cancellable exponential backoff retrier
		}()
	}

	if !disableTelemetry {
		go metric.SendHeartBeat(rootCtx, time.Minute*time.Duration(cnsconfig.TelemetrySettings.HeartBeatIntervalInMins), homeAzMonitor, cnsconfig.ChannelMode)
		go httpRemoteRestService.SendNCSnapShotPeriodically(rootCtx, cnsconfig.TelemetrySettings.SnapshotIntervalInMins)
	}

	// If CNS is running on managed DNC mode
	if config.ChannelMode == cns.Managed {
		if privateEndpoint == "" || infravnet == "" || nodeID == "" {
			logger.Errorf("[Azure CNS] Missing required values to run in managed mode: PrivateEndpoint: %s InfrastructureNetworkID: %s NodeID: %s",
				privateEndpoint,
				infravnet,
				nodeID)
			return
		}

		httpRemoteRestService.SetOption(acn.OptPrivateEndpoint, privateEndpoint)
		httpRemoteRestService.SetOption(acn.OptInfrastructureNetworkID, infravnet)
		httpRemoteRestService.SetOption(acn.OptNodeID, nodeID)

		// Passing in the default http client that already implements Do function
		standardClient := http.DefaultClient

		registerErr := registerNode(rootCtx, standardClient, httpRemoteRestService, privateEndpoint, infravnet, nodeID, nmaClient)
		if registerErr != nil {
			logger.Errorf("[Azure CNS] Registering Node failed with error: %v PrivateEndpoint: %s InfrastructureNetworkID: %s NodeID: %s",
				registerErr,
				privateEndpoint,
				infravnet,
				nodeID)
			return
		}
		go func(ep, vnet, node string) {
			// Periodically poll DNC for node updates
			tickerChannel := time.Tick(time.Duration(cnsconfig.ManagedSettings.NodeSyncIntervalInSeconds) * time.Second)
			for {
				<-tickerChannel
				httpRemoteRestService.SyncNodeStatus(ep, vnet, node, json.RawMessage{})
			}
		}(privateEndpoint, infravnet, nodeID)
	}

	if config.ChannelMode == cns.AzureHost {
		// at this point, rest service is running. We can now start serving new requests. So call StartNodeSubnet, which
		// will fetch secondary IPs and generate conflist. Do not move this all before rest service start - this will cause
		// CNI to start sending requests, and if the service doesn't start successfully, the requests will fail.
		httpRemoteRestService.StartNodeSubnet(rootCtx)
	}

	// mark the service as "ready"
	close(readyCh)
	// block until process exiting
	<-rootCtx.Done()

	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.DeleteDefaultExtNetwork(); err == nil {
			logger.Printf("[Azure CNS] Successfully deleted default ext network")
		} else {
			logger.Printf("[Azure CNS] Failed to delete default ext network due to error: %v", err)
		}
	}

	logger.Printf("stop cns service")
	// Cleanup.
	if httpRemoteRestService != nil {
		httpRemoteRestService.Stop()
	}

	if err = lockclient.Unlock(); err != nil {
		logger.Errorf("lockclient cns unlock error:%v", err)
	}

	logger.Printf("CNS exited")
	logger.Close()
}

// Poll CRD until it's set and update PluginManager
func pollNodeInfoCRDAndUpdatePlugin(ctx context.Context, zlog *zap.Logger, pluginManager *deviceplugin.PluginManager) error {
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Errorf("Failed to get kubeconfig for request controller: %v", err)
		return errors.Wrap(err, "failed to get kubeconfig")
	}
	kubeConfig.UserAgent = "azure-cns-" + version

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build clientset")
	}

	nodeName, err := configuration.NodeName()
	if err != nil {
		return errors.Wrap(err, "failed to get NodeName")
	}

	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get node %s", nodeName)
	}

	// check the Node labels for Swift V2
	if _, ok := node.Labels[configuration.LabelNodeSwiftV2]; !ok {
		zlog.Info("Node is not labeled for Swift V2, skipping polling nodeinfo crd")
		return nil
	}

	directcli, err := client.New(kubeConfig, client.Options{Scheme: multitenancy.Scheme})
	if err != nil {
		return errors.Wrap(err, "failed to create ctrl client")
	}

	nodeInfoCli := multitenancy.NodeInfoClient{
		Cli: directcli,
	}

	ticker := time.NewTicker(defaultNodeInfoCRDPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			zlog.Info("Polling context canceled, exiting")
			return nil
		case <-ticker.C:
			// Fetch the CRD status
			nodeInfo, err := nodeInfoCli.Get(ctx, node.Name)
			if err != nil {
				zlog.Error("Error fetching nodeinfo CRD", zap.Error(err))
				return errors.Wrap(err, "failed to get nodeinfo crd")
			}

			// Check if the status is set
			if !cmp.Equal(nodeInfo.Status, mtv1alpha1.NodeInfoStatus{}) && len(nodeInfo.Status.DeviceInfos) > 0 {
				// Create a map to count devices by type
				deviceCounts := map[mtv1alpha1.DeviceType]int{
					mtv1alpha1.DeviceTypeVnetNIC:       0,
					mtv1alpha1.DeviceTypeInfiniBandNIC: 0,
				}

				// Aggregate device counts from the CRD
				for _, deviceInfo := range nodeInfo.Status.DeviceInfos {
					switch deviceInfo.DeviceType {
					case mtv1alpha1.DeviceTypeVnetNIC, mtv1alpha1.DeviceTypeInfiniBandNIC:
						deviceCounts[deviceInfo.DeviceType]++
					default:
						zlog.Error("Unknown device type", zap.String("deviceType", string(deviceInfo.DeviceType)))
					}
				}

				// Update the plugin manager with device counts
				for deviceType, count := range deviceCounts {
					pluginManager.TrackDevices(deviceType, count)
				}

				// Exit polling loop once the CRD status is successfully processed
				return nil
			}
		}
	}
}

func InitializeMultiTenantController(ctx context.Context, httpRestService cns.HTTPService, cnsconfig configuration.CNSConfig) error {
	var multiTenantController multitenantcontroller.RequestController
	kubeConfig, err := ctrl.GetConfig()
	kubeConfig.UserAgent = fmt.Sprintf("azure-cns-%s", version)
	if err != nil {
		return err
	}

	// convert interface type to implementation type
	httpRestServiceImpl, ok := httpRestService.(*restserver.HTTPRestService)
	if !ok {
		logger.Errorf("Failed to convert interface httpRestService to implementation: %v", httpRestService)
		return fmt.Errorf("Failed to convert interface httpRestService to implementation: %v",
			httpRestService)
	}

	// Set orchestrator type
	orchestrator := cns.SetOrchestratorTypeRequest{
		OrchestratorType: cns.Kubernetes,
	}
	httpRestServiceImpl.SetNodeOrchestrator(&orchestrator)

	// Create multiTenantController.
	multiTenantController, err = multitenantoperator.New(httpRestServiceImpl, kubeConfig)
	if err != nil {
		logger.Errorf("Failed to create multiTenantController:%v", err)
		return err
	}

	// Wait for multiTenantController to start.
	go func() {
		for {
			if err := multiTenantController.Start(ctx); err != nil {
				logger.Errorf("Failed to start multiTenantController: %v", err)
			} else {
				logger.Printf("Exiting multiTenantController")
				return
			}

			// Retry after 1sec
			time.Sleep(time.Second)
		}
	}()
	for {
		if multiTenantController.IsStarted() {
			logger.Printf("MultiTenantController is started")
			break
		}

		logger.Printf("Waiting for multiTenantController to start...")
		time.Sleep(time.Millisecond * 500)
	}

	// TODO: do we need this to be running?
	logger.Printf("Starting SyncHostNCVersion")
	go func() {
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				timedCtx, cancel := context.WithTimeout(ctx, time.Duration(cnsconfig.SyncHostNCVersionIntervalMs)*time.Millisecond)
				httpRestServiceImpl.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode)
				cancel()
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

type nodeNetworkConfigGetter interface {
	Get(context.Context) (*v1alpha.NodeNetworkConfig, error)
}

type ipamStateReconciler interface {
	ReconcileIPAMStateForSwift(ncRequests []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) cnstypes.ResponseCode
}

// TODO(rbtr) where should this live??
// reconcileInitialCNSState initializes cns by passing pods and a CreateNetworkContainerRequest
func reconcileInitialCNSState(ctx context.Context, cli nodeNetworkConfigGetter, ipamReconciler ipamStateReconciler, podInfoByIPProvider cns.PodInfoByIPProvider, isSwiftV2 bool) error {
	// Get nnc using direct client
	nnc, err := cli.Get(ctx)
	if err != nil {
		if crd.IsNotDefined(err) {
			return errors.Wrap(err, "failed to init CNS state: NNC CRD is not defined")
		}
		if apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to init CNS state: NNC not found")
		}
		return errors.Wrap(err, "failed to init CNS state: failed to get NNC CRD")
	}

	logger.Printf("Retrieved NNC: %+v", nnc)
	if !nnc.DeletionTimestamp.IsZero() {
		return errors.New("failed to init CNS state: NNC is being deleted")
	}

	// If there are no NCs, we can't initialize our state and we should fail out.
	if len(nnc.Status.NetworkContainers) == 0 {
		return errors.New("failed to init CNS state: no NCs found in NNC CRD")
	}

	// Get previous PodInfo state from podInfoByIPProvider
	podInfoByIP, err := podInfoByIPProvider.PodInfoByIP()
	if err != nil {
		return errors.Wrap(err, "provider failed to provide PodInfoByIP")
	}

	ncReqs := make([]*cns.CreateNetworkContainerRequest, len(nnc.Status.NetworkContainers))

	// For each NC, we need to create a CreateNetworkContainerRequest and use it to rebuild our state.
	for i := range nnc.Status.NetworkContainers {
		var (
			ncRequest *cns.CreateNetworkContainerRequest
			err       error
		)
		switch nnc.Status.NetworkContainers[i].AssignmentMode { //nolint:exhaustive // skipping dynamic case
		case v1alpha.Static:
			ncRequest, err = nncctrl.CreateNCRequestFromStaticNC(nnc.Status.NetworkContainers[i], isSwiftV2)
		default: // For backward compatibility, default will be treated as Dynamic too.
			ncRequest, err = nncctrl.CreateNCRequestFromDynamicNC(nnc.Status.NetworkContainers[i])
		}

		if err != nil {
			return errors.Wrapf(err, "failed to convert NNC status to network container request, "+
				"assignmentMode: %s", nnc.Status.NetworkContainers[i].AssignmentMode)
		}

		ncReqs[i] = ncRequest
	}

	// Call cnsclient init cns passing those two things.
	if err := restserver.ResponseCodeToError(ipamReconciler.ReconcileIPAMStateForSwift(ncReqs, podInfoByIP, nnc)); err != nil {
		return errors.Wrap(err, "failed to reconcile CNS IPAM state")
	}

	return nil
}

// InitializeCRDState builds and starts the CRD controllers.
//
//nolint:gocyclo // legacy
func InitializeCRDState(ctx context.Context, z *zap.Logger, httpRestService cns.HTTPService, cnsconfig *configuration.CNSConfig) error {
	// convert interface type to implementation type
	httpRestServiceImplementation, ok := httpRestService.(*restserver.HTTPRestService)
	if !ok {
		logger.Errorf("[Azure CNS] Failed to convert interface httpRestService to implementation: %v", httpRestService)
		return fmt.Errorf("[Azure CNS] Failed to convert interface httpRestService to implementation: %v",
			httpRestService)
	}

	// Set orchestrator type
	orchestrator := cns.SetOrchestratorTypeRequest{
		OrchestratorType: cns.KubernetesCRD,
	}
	httpRestServiceImplementation.SetNodeOrchestrator(&orchestrator)

	// build default clientset.
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to get kubeconfig for request controller: %v", err)
		return errors.Wrap(err, "failed to get kubeconfig")
	}
	kubeConfig.UserAgent = fmt.Sprintf("azure-cns-%s", version)

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to build clientset")
	}

	// get nodename for scoping kube requests to node.
	nodeName, err := configuration.NodeName()
	if err != nil {
		return errors.Wrap(err, "failed to get NodeName")
	}

	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get node %s", nodeName)
	}

	// check the Node labels for Swift V2
	if _, ok := node.Labels[configuration.LabelNodeSwiftV2]; ok {
		cnsconfig.EnableSwiftV2 = true
		cnsconfig.WatchPods = true
		if nodeInfoErr := createOrUpdateNodeInfoCRD(ctx, kubeConfig, node); nodeInfoErr != nil {
			return errors.Wrap(nodeInfoErr, "error creating or updating nodeinfo crd")
		}
	}

	// perform state migration from CNI in case CNS is set to manage the endpoint state and has emty state
	if cnsconfig.EnableStateMigration && !httpRestServiceImplementation.EndpointStateStore.Exists() {
		if err = PopulateCNSEndpointState(httpRestServiceImplementation.EndpointStateStore); err != nil {
			return errors.Wrap(err, "failed to create CNS EndpointState From CNI")
		}
		// endpoint state needs tobe loaded in memory so the subsequent Delete calls remove the state and release the IPs.
		if err = httpRestServiceImplementation.EndpointStateStore.Read(restserver.EndpointStoreKey, &httpRestServiceImplementation.EndpointState); err != nil {
			return errors.Wrap(err, "failed to restore endpoint state")
		}
	}

	podInfoByIPProvider, err := getPodInfoByIPProvider(ctx, cnsconfig, httpRestServiceImplementation, clientset, nodeName)
	if err != nil {
		return errors.Wrap(err, "failed to initialize ip state")
	}

	// create scoped kube clients.
	directcli, err := client.New(kubeConfig, client.Options{Scheme: nodenetworkconfig.Scheme})
	if err != nil {
		return errors.Wrap(err, "failed to create ctrl client")
	}
	directnnccli := nodenetworkconfig.NewClient(directcli)
	if err != nil {
		return errors.Wrap(err, "failed to create NNC client")
	}
	// TODO(rbtr): nodename and namespace should be in the cns config
	directscopedcli := nncctrl.NewScopedClient(directnnccli, types.NamespacedName{Namespace: "kube-system", Name: nodeName})

	logger.Printf("Reconciling initial CNS state")
	// apiserver nnc might not be registered or api server might be down and crashloop backof puts us outside of 5-10 minutes we have for
	// aks addons to come up so retry a bit more aggresively here.
	// will retry 10 times maxing out at a minute taking about 8 minutes before it gives up.
	attempt := 0
	_ = retry.Do(func() error {
		attempt++
		logger.Printf("reconciling initial CNS state attempt: %d", attempt)
		err = reconcileInitialCNSState(ctx, directscopedcli, httpRestServiceImplementation, podInfoByIPProvider, cnsconfig.EnableSwiftV2)
		if err != nil {
			logger.Errorf("failed to reconcile initial CNS state, attempt: %d err: %v", attempt, err)
			nncInitFailure.Inc()
		}
		return errors.Wrap(err, "failed to initialize CNS state")
	}, retry.Context(ctx), retry.Delay(initCNSInitalDelay), retry.MaxDelay(time.Minute), retry.UntilSucceeded())
	logger.Printf("reconciled initial CNS state after %d attempts", attempt)
	hasNNCInitialized.Set(1)
	scheme := kuberuntime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil { //nolint:govet // intentional shadow
		return errors.Wrap(err, "failed to add corev1 to scheme")
	}
	if err = v1alpha.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add nodenetworkconfig/v1alpha to scheme")
	}
	if err = cssv1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add clustersubnetstate/v1alpha1 to scheme")
	}
	if err = mtv1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add multitenantpodnetworkconfig/v1alpha1 to scheme")
	}

	// Set Selector options on the Manager cache which are used
	// to perform *server-side* filtering of the cached objects. This is very important
	// for high node/pod count clusters, as it keeps us from watching objects at the
	// whole cluster scope when we are only interested in the Node's scope.
	cacheOpts := cache.Options{
		Scheme: scheme,
		ByObject: map[client.Object]cache.ByObject{
			&v1alpha.NodeNetworkConfig{}: {
				Namespaces: map[string]cache.Config{
					"kube-system": {FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": nodeName})},
				},
			},
		},
	}

	if cnsconfig.WatchPods {
		cacheOpts.ByObject[&corev1.Pod{}] = cache.ByObject{
			Field: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
		}
	}

	if cnsconfig.EnableSubnetScarcity {
		cacheOpts.ByObject[&cssv1alpha1.ClusterSubnetState{}] = cache.ByObject{
			Namespaces: map[string]cache.Config{
				"kube-system": {},
			},
		}
	}

	managerOpts := ctrlmgr.Options{
		Scheme:  scheme,
		Metrics: ctrlmetrics.Options{BindAddress: "0"},
		Cache:   cacheOpts,
		Logger:  zapr.NewLogger(z),
	}

	manager, err := ctrl.NewManager(kubeConfig, managerOpts)
	if err != nil {
		return errors.Wrap(err, "failed to create manager")
	}

	// this cachedscopedclient is built using the Manager's cached client, which is
	// NOT SAFE TO USE UNTIL THE MANAGER IS STARTED!
	// This is okay because it is only used to build the IPAMPoolMonitor, which does not
	// attempt to use the client until it has received a NodeNetworkConfig to update, and
	// that can only happen once the Manager has started and the NodeNetworkConfig
	// reconciler has pushed the Monitor a NodeNetworkConfig.
	cachedscopedcli := nncctrl.NewScopedClient(nodenetworkconfig.NewClient(manager.GetClient()), types.NamespacedName{Namespace: "kube-system", Name: nodeName})

	// Build the IPAM Pool monitor
	var poolMonitor cns.IPAMPoolMonitor
	cssCh := make(chan cssv1alpha1.ClusterSubnetState)
	ipDemandCh := make(chan int)
	if cnsconfig.EnableIPAMv2 {
		cssSrc := func(context.Context) ([]cssv1alpha1.ClusterSubnetState, error) { return nil, nil }
		if cnsconfig.EnableSubnetScarcity {
			cssSrc = clustersubnetstate.NewClient(manager.GetClient()).List
		}
		nncCh := make(chan v1alpha.NodeNetworkConfig)
		pmv2 := ipampoolv2.NewMonitor(z, httpRestServiceImplementation, cachedscopedcli, ipDemandCh, nncCh, cssCh)
		obs := metrics.NewLegacyMetricsObserver(httpRestService.GetPodIPConfigState, cachedscopedcli.Get, cssSrc)
		pmv2.WithLegacyMetricsObserver(obs)
		poolMonitor = pmv2.AsV1(nncCh)
	} else {
		poolOpts := ipampool.Options{
			RefreshDelay: poolIPAMRefreshRateInMilliseconds * time.Millisecond,
		}
		poolMonitor = ipampool.NewMonitor(httpRestServiceImplementation, cachedscopedcli, cssCh, &poolOpts)
	}

	// Start building the NNC Reconciler

	// get CNS Node IP to compare NC Node IP with this Node IP to ensure NCs were created for this node
	nodeIP := configuration.NodeIP()
	nncReconciler := nncctrl.NewReconciler(httpRestServiceImplementation, poolMonitor, nodeIP, cnsconfig.EnableSwiftV2)
	// pass Node to the Reconciler for Controller xref
	// IPAMv1 - reconcile only status changes (where generation doesn't change).
	// IPAMv2 - reconcile all updates.
	filterGenerationChange := !cnsconfig.EnableIPAMv2
	if err := nncReconciler.SetupWithManager(manager, node, filterGenerationChange); err != nil { //nolint:govet // intentional shadow
		return errors.Wrapf(err, "failed to setup nnc reconciler with manager")
	}

	if cnsconfig.EnableSubnetScarcity {
		// ClusterSubnetState reconciler
		cssReconciler := cssctrl.New(cssCh)
		if err := cssReconciler.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup css reconciler with manager")
		}
	}

	// TODO: add pod listeners based on Swift V1 vs MT/V2 configuration
	if cnsconfig.WatchPods {
		pw := podctrl.New(z)
		if cnsconfig.EnableIPAMv2 {
			hostNetworkListOpt := &client.ListOptions{FieldSelector: fields.SelectorFromSet(fields.Set{"spec.hostNetwork": "false"})} // filter only podsubnet pods
			// don't relist pods more than every 500ms
			limit := rate.NewLimiter(rate.Every(500*time.Millisecond), 1) //nolint:gomnd // clearly 500ms
			pw.With(pw.NewNotifierFunc(hostNetworkListOpt, limit, ipampoolv2.PodIPDemandListener(ipDemandCh)))
		}
		if err := pw.SetupWithManager(ctx, manager); err != nil {
			return errors.Wrapf(err, "failed to setup pod watcher with manager")
		}
	}

	if cnsconfig.EnableSwiftV2 {
		if err := mtpncctrl.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup mtpnc reconciler with manager")
		}
		// if SWIFT v2 is enabled on CNS, attach multitenant middleware to rest service
		// switch here for AKS(K8s) swiftv2 middleware to process IP configs requests
		swiftV2Middleware := &middlewares.K8sSWIFTv2Middleware{Cli: manager.GetClient()}
		httpRestService.AttachIPConfigsHandlerMiddleware(swiftV2Middleware)
	}

	// start the pool Monitor before the Reconciler, since it needs to be ready to receive an
	// NodeNetworkConfig update by the time the Reconciler tries to send it.
	go func() {
		logger.Printf("Starting IPAM Pool Monitor")
		if e := poolMonitor.Start(ctx); e != nil {
			logger.Errorf("[Azure CNS] Failed to start pool monitor with err: %v", e)
		}
	}()
	logger.Printf("initialized and started IPAM pool monitor")

	// Start the Manager which starts the reconcile loop.
	// The Reconciler will send an initial NodeNetworkConfig update to the PoolMonitor, starting the
	// Monitor's internal loop.
	go func() {
		logger.Printf("Starting controller-manager.")
		for {
			if err := manager.Start(ctx); err != nil {
				logger.Errorf("Failed to start controller-manager: %v", err)
				// retry to start the request controller
				// inc the managerStartFailures metric for failure tracking
				managerStartFailures.Inc()
			} else {
				logger.Printf("Stopped controller-manager.")
				return
			}
			time.Sleep(time.Second) // TODO(rbtr): make this exponential backoff
		}
	}()
	logger.Printf("Initialized controller-manager.")
	for {
		logger.Printf("Waiting for NodeNetworkConfig reconciler to start.")
		// wait for the Reconciler to run once on a NNC that was made for this Node.
		// the nncReadyCtx has a timeout of 15 minutes, after which we will consider
		// this false and the NNC Reconciler stuck/failed, log and retry.
		nncReadyCtx, cancel := context.WithTimeout(ctx, 15*time.Minute) // nolint // it will time out and not leak
		if started, err := nncReconciler.Started(nncReadyCtx); !started {
			logger.Errorf("NNC reconciler has not started, does the NNC exist? err: %v", err)
			nncReconcilerStartFailures.Inc()
			continue
		}
		logger.Printf("NodeNetworkConfig reconciler has started.")
		cancel()
		break
	}

	go func() {
		logger.Printf("Starting SyncHostNCVersion loop.")
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				timedCtx, cancel := context.WithTimeout(ctx, time.Duration(cnsconfig.SyncHostNCVersionIntervalMs)*time.Millisecond)
				httpRestServiceImplementation.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode)
				cancel()
			case <-ctx.Done():
				logger.Printf("Stopping SyncHostNCVersion loop.")
				return
			}
		}
	}()
	logger.Printf("Initialized SyncHostNCVersion loop.")
	return nil
}

// getPodInfoByIPProvider returns a PodInfoByIPProvider that reads endpoint state from the configured source
func getPodInfoByIPProvider(
	ctx context.Context,
	cnsconfig *configuration.CNSConfig,
	httpRestServiceImplementation *restserver.HTTPRestService,
	clientset *kubernetes.Clientset,
	nodeName string,
) (podInfoByIPProvider cns.PodInfoByIPProvider, err error) {
	switch {
	case cnsconfig.ManageEndpointState:
		logger.Printf("Initializing from self managed endpoint store")
		podInfoByIPProvider, err = cnspodprovider.New(httpRestServiceImplementation.EndpointStateStore) // get reference to endpoint state store from rest server
		if err != nil {
			if errors.Is(err, store.ErrKeyNotFound) {
				logger.Printf("[Azure CNS] No endpoint state found, skipping initializing CNS state")
			} else {
				return podInfoByIPProvider, errors.Wrap(err, "failed to create CNS PodInfoProvider")
			}
		}
	default:
		logger.Printf("Initializing from CNI")
		podInfoByIPProvider, err = cnipodprovider.New()
		if err != nil {
			return podInfoByIPProvider, errors.Wrap(err, "failed to create CNI PodInfoProvider")
		}
	}
	return podInfoByIPProvider, nil
}

// createOrUpdateNodeInfoCRD polls imds to learn the VM Unique ID and then creates or updates the NodeInfo CRD
// with that vm unique ID
func createOrUpdateNodeInfoCRD(ctx context.Context, restConfig *rest.Config, node *corev1.Node) error {
	imdsCli := imds.NewClient()
	vmUniqueID, err := imdsCli.GetVMUniqueID(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting vm unique ID from imds")
	}

	directcli, err := client.New(restConfig, client.Options{Scheme: multitenancy.Scheme})
	if err != nil {
		return errors.Wrap(err, "failed to create ctrl client")
	}

	nodeInfoCli := multitenancy.NodeInfoClient{
		Cli: directcli,
	}

	nodeInfo := &mtv1alpha1.NodeInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Spec: mtv1alpha1.NodeInfoSpec{
			VMUniqueID: vmUniqueID,
		},
	}

	if err := controllerutil.SetOwnerReference(node, nodeInfo, multitenancy.Scheme); err != nil {
		return errors.Wrap(err, "failed to set nodeinfo owner reference to node")
	}

	if err := nodeInfoCli.CreateOrUpdate(ctx, nodeInfo, "azure-cns"); err != nil {
		return errors.Wrap(err, "error ensuring nodeinfo CRD exists and is up-to-date")
	}

	return nil
}

// PopulateCNSEndpointState initilizes CNS Endpoint State by Migrating the CNI state.
func PopulateCNSEndpointState(endpointStateStore store.KeyValueStore) error {
	logger.Printf("State Migration is enabled")
	endpointState, err := cnspodprovider.MigrateCNISate()
	if err != nil {
		return errors.Wrap(err, "failed to create CNS Endpoint state from CNI")
	}
	err = endpointStateStore.Write(restserver.EndpointStoreKey, endpointState)
	if err != nil {
		return fmt.Errorf("failed to write endpoint state to store: %w", err)
	}
	return nil
}
