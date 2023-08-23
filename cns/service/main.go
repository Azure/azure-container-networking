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
	"github.com/Azure/azure-container-networking/cnm/ipam"
	"github.com/Azure/azure-container-networking/cnm/network"
	"github.com/Azure/azure-container-networking/cns"
	cnscli "github.com/Azure/azure-container-networking/cns/cmd/cli"
	"github.com/Azure/azure-container-networking/cns/cniconflist"
	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/healthserver"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/ipampool"
	cssctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/clustersubnetstate"
	nncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/cns/kubecontroller/podwatcher"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/multitenantoperator"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
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
	"github.com/avast/retry-go/v3"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	// Service name.
	name                              = "azure-cns"
	pluginName                        = "azure-vnet"
	endpointStoreName                 = "azure-endpoints"
	endpointStoreLocation             = "/var/run/azure-cns/"
	defaultCNINetworkConfigFileName   = "10-azure.conflist"
	dncApiVersion                     = "?api-version=2018-03-01"
	poolIPAMRefreshRateInMilliseconds = 1000

	// 720 * acn.FiveSeconds sec sleeps = 1Hr
	maxRetryNodeRegister = 720
	initCNSInitalDelay   = 10 * time.Second

	// envVarEnableCNIConflistGeneration enables cni conflist generation if set (value doesn't matter)
	envVarEnableCNIConflistGeneration = "CNS_ENABLE_CNI_CONFLIST_GENERATION"
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
		Name:         acn.OptStartAzureCNM,
		Shorthand:    acn.OptStartAzureCNMAlias,
		Description:  "Start Azure-CNM if flag is set",
		Type:         "bool",
		DefaultValue: false,
	},
	{
		Name:         acn.OptVersion,
		Shorthand:    acn.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
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
		Description:  "Create default external network for windows platform with the specified type (l2bridge or l2tunnel)",
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
		Name:         acn.OptStoreFileLocation,
		Shorthand:    acn.OptStoreFileLocationAlias,
		Description:  "Set store file absolute path",
		Type:         "string",
		DefaultValue: platform.CNMRuntimePath,
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

	go func() {
		// Wait until receiving a signal.
		select {
		case sig := <-sigCh:
			log.Errorf("caught exit signal %v, exiting", sig)
		case err := <-rootErrCh:
			log.Errorf("unhandled error %v, exiting", err)
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

// RegisterNode - Tries to register node with DNC when CNS is started in managed DNC mode
func registerNode(httpc *http.Client, httpRestService cns.HTTPService, dncEP, infraVnet, nodeID string, ni NodeInterrogator) error {
	logger.Printf("[Azure CNS] Registering node %s with Infrastructure Network: %s PrivateEndpoint: %s", nodeID, infraVnet, dncEP)

	var (
		numCPU              = runtime.NumCPU()
		url                 = fmt.Sprintf(acn.RegisterNodeURLFmt, dncEP, infraVnet, nodeID, dncApiVersion)
		nodeRegisterRequest cns.NodeRegisterRequest
	)

	nodeRegisterRequest.NumCores = numCPU
	supportedApis, retErr := ni.SupportedAPIs(context.TODO())

	if retErr != nil {
		logger.Errorf("[Azure CNS] Failed to retrieve SupportedApis from NMagent of node %s with Infrastructure Network: %s PrivateEndpoint: %s",
			nodeID, infraVnet, dncEP)
		return retErr
	}

	// To avoid any null-pointer deferencing errors.
	if supportedApis == nil {
		supportedApis = []string{}
	}

	nodeRegisterRequest.NmAgentSupportedApis = supportedApis

	// CNS tries to register Node for maximum of an hour.
	err := retry.Do(func() error {
		return sendRegisterNodeRequest(httpc, httpRestService, nodeRegisterRequest, url)
	}, retry.Delay(acn.FiveSeconds), retry.Attempts(maxRetryNodeRegister), retry.DelayType(retry.FixedDelay))

	return errors.Wrap(err, fmt.Sprintf("[Azure CNS] Failed to register node %s after maximum reties for an hour with Infrastructure Network: %s PrivateEndpoint: %s",
		nodeID, infraVnet, dncEP))
}

// sendRegisterNodeRequest func helps in registering the node until there is an error.
func sendRegisterNodeRequest(httpc *http.Client, httpRestService cns.HTTPService, nodeRegisterRequest cns.NodeRegisterRequest, registerURL string) error {
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode(nodeRegisterRequest)
	if err != nil {
		log.Errorf("[Azure CNS] Failed to register node while encoding json failed with non-retriable err %v", err)
		return errors.Wrap(retry.Unrecoverable(err), "failed to sendRegisterNodeRequest")
	}

	response, err := httpc.Post(registerURL, "application/json", &body)
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to register node with retriable err: %+v", err)
		return errors.Wrap(err, "failed to sendRegisterNodeRequest")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		err = fmt.Errorf("[Azure CNS] Failed to register node, DNC replied with http status code %s", strconv.Itoa(response.StatusCode))
		logger.Errorf(err.Error())
		return errors.Wrap(err, "failed to sendRegisterNodeRequest")
	}

	var req cns.SetOrchestratorTypeRequest
	err = json.NewDecoder(response.Body).Decode(&req)
	if err != nil {
		log.Errorf("[Azure CNS] decoding Node Resgister response json failed with err %v", err)
		return errors.Wrap(err, "failed to sendRegisterNodeRequest")
	}
	httpRestService.SetNodeOrchestrator(&req)

	logger.Printf("[Azure CNS] Node Registered")
	return nil
}

func startTelemetryService(ctx context.Context) {
	var config aitelemetry.AIConfig

	err := telemetry.CreateAITelemetryHandle(config, false, false, false)
	if err != nil {
		log.Errorf("AI telemetry handle creation failed..:%w", err)
		return
	}

	tbtemp := telemetry.NewTelemetryBuffer()
	//nolint:errcheck // best effort to cleanup leaked pipe/socket before start
	tbtemp.Cleanup(telemetry.FdName)

	tb := telemetry.NewTelemetryBuffer()
	err = tb.StartServer()
	if err != nil {
		log.Errorf("Telemetry service failed to start: %w", err)
		return
	}
	tb.PushData(rootCtx)
}

// Main is the entry point for CNS.
func main() {
	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)

	environment := acn.GetArg(acn.OptEnvironment).(string)
	url := acn.GetArg(acn.OptAPIServerURL).(string)
	cniPath := acn.GetArg(acn.OptNetPluginPath).(string)
	cniConfigFile := acn.GetArg(acn.OptNetPluginConfigFile).(string)
	cnsURL := acn.GetArg(acn.OptCnsURL).(string)
	logLevel := acn.GetArg(acn.OptLogLevel).(int)
	logTarget := acn.GetArg(acn.OptLogTarget).(int)
	logDirectory := acn.GetArg(acn.OptLogLocation).(string)
	ipamQueryUrl := acn.GetArg(acn.OptIpamQueryUrl).(string)
	ipamQueryInterval := acn.GetArg(acn.OptIpamQueryInterval).(int)

	startCNM := acn.GetArg(acn.OptStartAzureCNM).(bool)
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

	// start the health server
	z, _ := zap.NewProduction()
	go healthserver.Start(z, cnsconfig.MetricsBindAddress)

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

	homeAzMonitor := restserver.NewHomeAzMonitor(nmaClient, time.Duration(cnsconfig.AZRSettings.PopulateHomeAzCacheRetryIntervalSecs)*time.Second)
	if cnsconfig.AZRSettings.EnableAZR {
		logger.Printf("start the goroutine for refreshing homeAz")
		homeAzMonitor.Start()
	}

	if cnsconfig.ChannelMode == cns.Managed {
		config.ChannelMode = cns.Managed
		privateEndpoint = cnsconfig.ManagedSettings.PrivateEndpoint
		infravnet = cnsconfig.ManagedSettings.InfrastructureNetworkID
		nodeID = cnsconfig.ManagedSettings.NodeID
	} else if cnsconfig.ChannelMode == cns.CRD {
		config.ChannelMode = cns.CRD
	} else if cnsconfig.ChannelMode == cns.MultiTenantCRD {
		config.ChannelMode = cns.MultiTenantCRD
	} else if acn.GetArg(acn.OptManaged).(bool) {
		config.ChannelMode = cns.Managed
	}

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
	}

	if telemetryDaemonEnabled {
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
		log.Printf("Error initializing file lock:%v", err)
		return
	}

	// Create the key value store.
	storeFileName := storeFileLocation + name + ".json"
	config.Store, err = store.NewJsonFileStore(storeFileName, lockclient)
	if err != nil {
		logger.Errorf("Failed to create store file: %s, due to error %v\n", storeFileName, err)
		return
	}

	// Initialize endpoint state store if cns is managing endpoint state.
	if cnsconfig.ManageEndpointState {
		log.Printf("[Azure CNS] Configured to manage endpoints state")
		endpointStoreLock, err := processlock.NewFileLock(platform.CNILockPath + endpointStoreName + store.LockExtension) // nolint
		if err != nil {
			log.Printf("Error initializing endpoint state file lock:%v", err)
			return
		}
		defer endpointStoreLock.Unlock() // nolint

		err = platform.CreateDirectory(endpointStoreLocation)
		if err != nil {
			logger.Errorf("Failed to create File Store directory %s, due to Error:%v", storeFileLocation, err.Error())
			return
		}
		// Create the key value store.
		storeFileName := endpointStoreLocation + endpointStoreName + ".json"
		endpointStateStore, err = store.NewJsonFileStore(storeFileName, endpointStoreLock)
		if err != nil {
			logger.Errorf("Failed to create endpoint state store file: %s, due to error %v\n", storeFileName, err)
			return
		}
	}

	wsProxy := wireserver.Proxy{
		Host:       cnsconfig.WireserverIP,
		HTTPClient: &http.Client{},
	}

	httpRestService, err := restserver.NewHTTPRestService(&config, &wireserver.Client{HTTPClient: &http.Client{}}, &wsProxy, nmaClient,
		endpointStateStore, conflistGenerator, homeAzMonitor)
	if err != nil {
		logger.Errorf("Failed to create CNS object, err:%v.\n", err)
		return
	}

	// Set CNS options.
	httpRestService.SetOption(acn.OptCnsURL, cnsURL)
	httpRestService.SetOption(acn.OptNetPluginPath, cniPath)
	httpRestService.SetOption(acn.OptNetPluginConfigFile, cniConfigFile)
	httpRestService.SetOption(acn.OptCreateDefaultExtNetworkType, createDefaultExtNetworkType)
	httpRestService.SetOption(acn.OptHttpConnectionTimeout, httpConnectionTimeout)
	httpRestService.SetOption(acn.OptHttpResponseHeaderTimeout, httpResponseHeaderTimeout)
	httpRestService.SetOption(acn.OptProgramSNATIPTables, cnsconfig.ProgramSNATIPTables)
	httpRestService.SetOption(acn.OptManageEndpointState, cnsconfig.ManageEndpointState)

	// Create default ext network if commandline option is set
	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.CreateDefaultExtNetwork(createDefaultExtNetworkType); err == nil {
			logger.Printf("[Azure CNS] Successfully created default ext network")
		} else {
			logger.Printf("[Azure CNS] Failed to create default ext network due to error: %v", err)
			return
		}
	}

	logger.Printf("[Azure CNS] Initialize HTTPRestService")
	if httpRestService != nil {
		if cnsconfig.UseHTTPS {
			config.TlsSettings = localtls.TlsSettings{
				TLSSubjectName:                     cnsconfig.TLSSubjectName,
				TLSCertificatePath:                 cnsconfig.TLSCertificatePath,
				TLSPort:                            cnsconfig.TLSPort,
				KeyVaultURL:                        cnsconfig.KeyVaultSettings.URL,
				KeyVaultCertificateName:            cnsconfig.KeyVaultSettings.CertificateName,
				MSIResourceID:                      cnsconfig.MSISettings.ResourceID,
				KeyVaultCertificateRefreshInterval: time.Duration(cnsconfig.KeyVaultSettings.RefreshIntervalInHrs) * time.Hour,
			}
		}

		err = httpRestService.Init(&config)
		if err != nil {
			logger.Errorf("Failed to init HTTPService, err:%v.\n", err)
			return
		}
	}

	// Setting the remote ARP MAC address to 12-34-56-78-9a-bc on windows for external traffic
	err = platform.SetSdnRemoteArpMacAddress()
	if err != nil {
		logger.Errorf("Failed to set remote ARP MAC address: %v", err)
		return
	}

	// We are only setting the PriorityVLANTag in 'cns.Direct' mode, because it neatly maps today, to 'isUsingMultitenancy'
	// In the future, we would want to have a better CNS flag, to explicitly say, this CNS is using multitenancy
	if config.ChannelMode == cns.Direct {
		// Set Mellanox adapter's PriorityVLANTag value to 3 if adapter exists
		// reg key value for PriorityVLANTag = 3  --> Packet priority and VLAN enabled
		// for more details goto https://docs.nvidia.com/networking/display/winof2v230/Configuring+the+Driver+Registry+Keys#ConfiguringtheDriverRegistryKeys-GeneralRegistryKeysGeneralRegistryKeys
		if platform.HasMellanoxAdapter() {
			go platform.MonitorAndSetMellanoxRegKeyPriorityVLANTag(rootCtx, cnsconfig.MellanoxMonitorIntervalSecs)
		}
	}

	// Initialze state in if CNS is running in CRD mode
	// State must be initialized before we start HTTPRestService
	if config.ChannelMode == cns.CRD {
		// Check the CNI statefile mount, and if the file is empty
		// stub an empty JSON object
		if err := cnireconciler.WriteObjectToCNIStatefile(); err != nil {
			logger.Errorf("Failed to write empty object to CNI state: %v", err)
			return
		}

		// We might be configured to reinitialize state from the CNI instead of the apiserver.
		// If so, we should check that the the CNI is new enough to support the state commands,
		// otherwise we fall back to the existing behavior.
		if cnsconfig.InitializeFromCNI {
			var isGoodVer bool
			isGoodVer, err = cnireconciler.IsDumpStateVer()
			if err != nil {
				logger.Errorf("error checking CNI ver: %v", err)
			}

			// override the prior config flag with the result of the ver check.
			cnsconfig.InitializeFromCNI = isGoodVer

			if cnsconfig.InitializeFromCNI {
				// Set the PodInfoVersion by initialization type, so that the
				// PodInfo maps use the correct key schema
				cns.GlobalPodInfoScheme = cns.InterfaceIDPodInfoScheme
			}
		}
		// If cns manageendpointstate is true, then cns maintains its own state and reconciles from it.
		// in this case, cns maintains state with containerid as key and so in-memory cache can lookup
		// and update based on container id.
		if cnsconfig.ManageEndpointState {
			cns.GlobalPodInfoScheme = cns.InterfaceIDPodInfoScheme
		}

		logger.Printf("Set GlobalPodInfoScheme %v (InitializeFromCNI=%t)", cns.GlobalPodInfoScheme, cnsconfig.InitializeFromCNI)

		err = InitializeCRDState(rootCtx, httpRestService, cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start CRD Controller, err:%v.\n", err)
			return
		}
	}

	// Initialize multi-tenant controller if the CNS is running in MultiTenantCRD mode.
	// It must be started before we start HTTPRestService.
	if config.ChannelMode == cns.MultiTenantCRD {
		err = InitializeMultiTenantController(rootCtx, httpRestService, *cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start multiTenantController, err:%v.\n", err)
			return
		}

	}

	logger.Printf("[Azure CNS] Start HTTP listener")
	if httpRestService != nil {
		if cnsconfig.EnablePprof {
			httpRestService.RegisterPProfEndpoints()
		}

		err = httpRestService.Start(&config)
		if err != nil {
			logger.Errorf("Failed to start CNS, err:%v.\n", err)
			return
		}
	}

	if !disableTelemetry {
		go logger.SendHeartBeat(rootCtx, cnsconfig.TelemetrySettings.HeartBeatIntervalInMins)
		go httpRestService.SendNCSnapShotPeriodically(rootCtx, cnsconfig.TelemetrySettings.SnapshotIntervalInMins)
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

		httpRestService.SetOption(acn.OptPrivateEndpoint, privateEndpoint)
		httpRestService.SetOption(acn.OptInfrastructureNetworkID, infravnet)
		httpRestService.SetOption(acn.OptNodeID, nodeID)

		registerErr := registerNode(acn.GetHttpClient(), httpRestService, privateEndpoint, infravnet, nodeID, nmaClient)
		if registerErr != nil {
			logger.Errorf("[Azure CNS] Resgistering Node failed with error: %v PrivateEndpoint: %s InfrastructureNetworkID: %s NodeID: %s",
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
				httpRestService.SyncNodeStatus(ep, vnet, node, json.RawMessage{})
			}
		}(privateEndpoint, infravnet, nodeID)
	}

	var (
		netPlugin     network.NetPlugin
		ipamPlugin    ipam.IpamPlugin
		lockclientCnm processlock.Interface
	)

	if startCNM {
		var pluginConfig acn.PluginConfig
		pluginConfig.Version = version

		// Create a channel to receive unhandled errors from the plugins.
		pluginConfig.ErrChan = make(chan error, 1)

		// Create network plugin.
		netPlugin, err = network.NewPlugin(&pluginConfig)
		if err != nil {
			logger.Errorf("Failed to create network plugin, err:%v.\n", err)
			return
		}

		// Create IPAM plugin.
		ipamPlugin, err = ipam.NewPlugin(&pluginConfig)
		if err != nil {
			logger.Errorf("Failed to create IPAM plugin, err:%v.\n", err)
			return
		}

		lockclientCnm, err = processlock.NewFileLock(platform.CNILockPath + pluginName + store.LockExtension)
		if err != nil {
			log.Printf("Error initializing file lock:%v", err)
			return
		}

		// Create the key value store.
		pluginStoreFile := storeFileLocation + pluginName + ".json"
		pluginConfig.Store, err = store.NewJsonFileStore(pluginStoreFile, lockclientCnm)
		if err != nil {
			logger.Errorf("Failed to create plugin store file %s, due to error : %v\n", pluginStoreFile, err)
			return
		}

		// Set plugin options.
		netPlugin.SetOption(acn.OptAPIServerURL, url)
		logger.Printf("Start netplugin\n")
		if err := netPlugin.Start(&pluginConfig); err != nil {
			logger.Errorf("Failed to create network plugin, err:%v.\n", err)
			return
		}

		ipamPlugin.SetOption(acn.OptEnvironment, environment)
		ipamPlugin.SetOption(acn.OptAPIServerURL, url)
		ipamPlugin.SetOption(acn.OptIpamQueryUrl, ipamQueryUrl)
		ipamPlugin.SetOption(acn.OptIpamQueryInterval, ipamQueryInterval)
		if err := ipamPlugin.Start(&pluginConfig); err != nil {
			logger.Errorf("Failed to create IPAM plugin, err:%v.\n", err)
			return
		}
	}

	// block until process exiting
	<-rootCtx.Done()

	if len(strings.TrimSpace(createDefaultExtNetworkType)) > 0 {
		if err := hnsclient.DeleteDefaultExtNetwork(); err == nil {
			logger.Printf("[Azure CNS] Successfully deleted default ext network")
		} else {
			logger.Printf("[Azure CNS] Failed to delete default ext network due to error: %v", err)
		}
	}

	if cnsconfig.AZRSettings.EnableAZR {
		logger.Printf("end the goroutine for refreshing homeAz")
		homeAzMonitor.Stop()
	}

	logger.Printf("stop cns service")
	// Cleanup.
	if httpRestService != nil {
		httpRestService.Stop()
	}

	if startCNM {
		logger.Printf("stop cnm plugin")
		if netPlugin != nil {
			netPlugin.Stop()
		}

		if ipamPlugin != nil {
			logger.Printf("stop ipam plugin")
			ipamPlugin.Stop()
		}

		if err = lockclientCnm.Unlock(); err != nil {
			log.Errorf("lockclient cnm unlock error:%v", err)
		}
	}

	if err = lockclient.Unlock(); err != nil {
		log.Errorf("lockclient cns unlock error:%v", err)
	}

	logger.Printf("CNS exited")
	logger.Close()
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
	ReconcileIPAMState(ncRequests []*cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) cnstypes.ResponseCode
}

// TODO(rbtr) where should this live??
// reconcileInitialCNSState initializes cns by passing pods and a CreateNetworkContainerRequest
func reconcileInitialCNSState(ctx context.Context, cli nodeNetworkConfigGetter, ipamReconciler ipamStateReconciler, podInfoByIPProvider cns.PodInfoByIPProvider) error {
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
			ncRequest, err = nncctrl.CreateNCRequestFromStaticNC(nnc.Status.NetworkContainers[i])
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
	if err := restserver.ResponseCodeToError(ipamReconciler.ReconcileIPAMState(ncReqs, podInfoByIP, nnc)); err != nil {
		return errors.Wrap(err, "failed to reconcile CNS IPAM state")
	}

	return nil
}

// InitializeCRDState builds and starts the CRD controllers.
func InitializeCRDState(ctx context.Context, httpRestService cns.HTTPService, cnsconfig *configuration.CNSConfig) error {
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

	var podInfoByIPProvider cns.PodInfoByIPProvider
	switch {
	case cnsconfig.ManageEndpointState:
		logger.Printf("Initializing from self managed endpoint store")
		podInfoByIPProvider, err = cnireconciler.NewCNSPodInfoProvider(httpRestServiceImplementation.EndpointStateStore) // get reference to endpoint state store from rest server
		if err != nil {
			if errors.Is(err, store.ErrKeyNotFound) {
				logger.Printf("[Azure CNS] No endpoint state found, skipping initializing CNS state")
			} else {
				return errors.Wrap(err, "failed to create CNS PodInfoProvider")
			}
		}
	case cnsconfig.InitializeFromCNI:
		logger.Printf("Initializing from CNI")
		podInfoByIPProvider, err = cnireconciler.NewCNIPodInfoProvider()
		if err != nil {
			return errors.Wrap(err, "failed to create CNI PodInfoProvider")
		}
	default:
		logger.Printf("Initializing from Kubernetes")
		podInfoByIPProvider = cns.PodInfoByIPProviderFunc(func() (map[string]cns.PodInfo, error) {
			pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{ //nolint:govet // ignore err shadow
				FieldSelector: "spec.nodeName=" + nodeName,
			})
			if err != nil {
				return nil, errors.Wrap(err, "failed to list Pods for PodInfoProvider")
			}
			podInfo, err := cns.KubePodsToPodInfoByIP(pods.Items)
			if err != nil {
				return nil, errors.Wrap(err, "failed to convert Pods to PodInfoByIP")
			}
			return podInfo, nil
		})
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
	err = retry.Do(func() error {
		attempt++
		logger.Printf("reconciling initial CNS state attempt: %d", attempt)
		err = reconcileInitialCNSState(ctx, directscopedcli, httpRestServiceImplementation, podInfoByIPProvider)
		if err != nil {
			logger.Errorf("failed to reconcile initial CNS state, attempt: %d err: %v", attempt, err)
		}
		return errors.Wrap(err, "failed to initialize CNS state")
	}, retry.Context(ctx), retry.Delay(initCNSInitalDelay), retry.MaxDelay(time.Minute))
	if err != nil {
		return err
	}
	logger.Printf("reconciled initial CNS state after %d attempts", attempt)

	scheme := kuberuntime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil { //nolint:govet // intentional shadow
		return errors.Wrap(err, "failed to add corev1 to scheme")
	}
	if err = v1alpha.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add nodenetworkconfig/v1alpha to scheme")
	}
	if err = v1alpha1.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "failed to add clustersubnetstate/v1alpha1 to scheme")
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
			&corev1.Pod{}: {
				Field: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
			},
		},
	}

	managerOpts := ctrlmgr.Options{
		Scheme:  scheme,
		Metrics: ctrlmetrics.Options{BindAddress: "0"},
		Cache:   cacheOpts,
		Logger:  ctrlzap.New(),
	}

	manager, err := ctrl.NewManager(kubeConfig, managerOpts)
	if err != nil {
		return errors.Wrap(err, "failed to create manager")
	}

	// Build the IPAM Pool monitor
	clusterSubnetStateChan := make(chan v1alpha1.ClusterSubnetState)

	// this cachedscopedclient is built using the Manager's cached client, which is
	// NOT SAFE TO USE UNTIL THE MANAGER IS STARTED!
	// This is okay because it is only used to build the IPAMPoolMonitor, which does not
	// attempt to use the client until it has received a NodeNetworkConfig to update, and
	// that can only happen once the Manager has started and the NodeNetworkConfig
	// reconciler has pushed the Monitor a NodeNetworkConfig.
	cachedscopedcli := nncctrl.NewScopedClient(nodenetworkconfig.NewClient(manager.GetClient()), types.NamespacedName{Namespace: "kube-system", Name: nodeName})

	poolOpts := ipampool.Options{
		RefreshDelay: poolIPAMRefreshRateInMilliseconds * time.Millisecond,
	}
	poolMonitor := ipampool.NewMonitor(httpRestServiceImplementation, cachedscopedcli, clusterSubnetStateChan, &poolOpts)
	httpRestServiceImplementation.IPAMPoolMonitor = poolMonitor

	// Start building the NNC Reconciler

	// get our Node so that we can xref it against the NodeNetworkConfig's to make sure that the
	// NNC is not stale and represents the Node we're running on.
	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get node %s", nodeName)
	}

	// check the Node labels for Swift V2
	if _, ok := node.Labels[configuration.LabelSwiftV2]; ok {
		cnsconfig.EnableSwiftV2 = true
		// TODO(rbtr): create the NodeInfo for Swift V2
	}

	// get CNS Node IP to compare NC Node IP with this Node IP to ensure NCs were created for this node
	nodeIP := configuration.NodeIP()

	// NodeNetworkConfig reconciler
	nncReconciler := nncctrl.NewReconciler(httpRestServiceImplementation, poolMonitor, nodeIP)
	// pass Node to the Reconciler for Controller xref
	if err := nncReconciler.SetupWithManager(manager, node); err != nil { //nolint:govet // intentional shadow
		return errors.Wrapf(err, "failed to setup nnc reconciler with manager")
	}

	if cnsconfig.EnableSubnetScarcity {
		// ClusterSubnetState reconciler
		cssReconciler := cssctrl.New(clusterSubnetStateChan)
		if err := cssReconciler.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup css reconciler with manager")
		}
	}

	// TODO: add pod listeners based on Swift V1 vs MT/V2 configuration
	if cnsconfig.WatchPods {
		pw := podwatcher.New(nodeName)
		if err := pw.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup pod watcher with manager")
		}
	}

	// adding some routes to the root service mux
	mux := httpRestServiceImplementation.Listener.GetMux()
	mux.Handle("/readyz", http.StripPrefix("/readyz", &healthz.Handler{}))
	if cnsconfig.EnablePprof {
		httpRestServiceImplementation.RegisterPProfEndpoints()
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
		nncReadyCtx, _ := context.WithTimeout(ctx, 15*time.Minute) //nolint // it will time out and not leak
		if started, err := nncReconciler.Started(nncReadyCtx); !started {
			log.Errorf("NNC reconciler has not started, does the NNC exist? err: %v", err)
			nncReconcilerStartFailures.Inc()
			continue
		}
		logger.Printf("NodeNetworkConfig reconciler has started.")
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
