// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
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
	"github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/healthserver"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/ipampool"
	cssctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/clustersubnetstate"
	nncctrl "github.com/Azure/azure-container-networking/cns/kubecontroller/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/multitenantoperator"
	"github.com/Azure/azure-container-networking/cns/nmagent"
	"github.com/Azure/azure-container-networking/cns/restserver"
	cnstypes "github.com/Azure/azure-container-networking/cns/types"
	"github.com/Azure/azure-container-networking/cns/wireserver"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/crd"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate"
	"github.com/Azure/azure-container-networking/crd/clustersubnetstate/api/v1alpha1"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig"
	"github.com/Azure/azure-container-networking/crd/nodenetworkconfig/api/v1alpha"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	localtls "github.com/Azure/azure-container-networking/server/tls"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/avast/retry-go/v3"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

const (
	// Service name.
	name                              = "azure-cns"
	pluginName                        = "azure-vnet"
	defaultCNINetworkConfigFileName   = "10-azure.conflist"
	dncApiVersion                     = "?api-version=2018-03-01"
	poolIPAMRefreshRateInMilliseconds = 1000

	// 720 * acn.FiveSeconds sec sleeps = 1Hr
	maxRetryNodeRegister = 720
	initCNSInitalDelay   = 10 * time.Second
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

// RegisterNode - Tries to register node with DNC when CNS is started in managed DNC mode
func registerNode(httpc *http.Client, httpRestService cns.HTTPService, dncEP, infraVnet, nodeID string) error {
	logger.Printf("[Azure CNS] Registering node %s with Infrastructure Network: %s PrivateEndpoint: %s", nodeID, infraVnet, dncEP)

	var (
		numCPU              = runtime.NumCPU()
		url                 = fmt.Sprintf(acn.RegisterNodeURLFmt, dncEP, infraVnet, nodeID, dncApiVersion)
		nodeRegisterRequest cns.NodeRegisterRequest
	)

	nodeRegisterRequest.NumCores = numCPU
	supportedApis, retErr := nmagent.GetNmAgentSupportedApis(httpc, "")

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

	logger.Printf("[Azure CNS] cmdLineConfigPath: %s", cmdLineConfigPath)
	cnsconfig, err := configuration.ReadConfig(cmdLineConfigPath)
	if err != nil {
		logger.Errorf("[Azure CNS] Error reading cns config: %v", err)
	}

	configuration.SetCNSConfigDefaults(cnsconfig)
	logger.Printf("[Azure CNS] Read config :%+v", cnsconfig)

	// start the health server
	z, _ := zap.NewProduction()
	go healthserver.Start(z, cnsconfig.MetricsBindAddress)

	if cnsconfig.WireserverIP != "" {
		nmagent.WireserverIP = cnsconfig.WireserverIP
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

		logger.InitAI(aiConfig, ts.DisableTrace, ts.DisableMetric, ts.DisableEvent)
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

	nmaclient, err := nmagent.NewClient("")
	if err != nil {
		logger.Errorf("Failed to start nmagent client due to error %v", err)
		return
	}

	// Initialize endpoint state store if cns is managing endpoint state.
	if cnsconfig.ManageEndpointState {
		lockclient, err = processlock.NewFileLock(platform.CNILockPath + "endpoints" + store.LockExtension)
		if err != nil {
			log.Printf("Error initializing endpoint state file lock:%v", err)
			return
		}

		// Create the key value store.
		storeFileName := storeFileLocation + "endpoints" + ".json"
		endpointStateStore, err = store.NewJsonFileStore(storeFileName, lockclient)
		if err != nil {
			logger.Errorf("Failed to create endpoint state store file: %s, due to error %v\n", storeFileName, err)
			return
		}
	}

	// Create CNS object.

	httpRestService, err := restserver.NewHTTPRestService(&config, &wireserver.Client{HTTPClient: &http.Client{}}, nmaclient, endpointStateStore)
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
		logger.Printf("Set GlobalPodInfoScheme %v (InitializeFromCNI=%t)", cns.GlobalPodInfoScheme, cnsconfig.InitializeFromCNI)

		err = InitializeCRDState(rootCtx, httpRestService, cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start CRD Controller, err:%v.\n", err)
			return
		}

		// Setting the remote ARP MAC address to 12-34-56-78-9a-bc on windows for external traffic
		err = platform.SetSdnRemoteArpMacAddress()
		if err != nil {
			logger.Errorf("Failed to set remote ARP MAC address: %v", err)
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

		// Setting the remote ARP MAC address to 12-34-56-78-9a-bc on windows for external traffic
		err = platform.SetSdnRemoteArpMacAddress()
		if err != nil {
			logger.Errorf("Failed to set remote ARP MAC address: %v", err)
			return
		}
	}

	logger.Printf("[Azure CNS] Start HTTP listener")
	if httpRestService != nil {
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

		registerErr := registerNode(acn.GetHttpClient(), httpRestService, privateEndpoint, infravnet, nodeID)
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

type ncStateReconciler interface {
	ReconcileNCState(ncRequest *cns.CreateNetworkContainerRequest, podInfoByIP map[string]cns.PodInfo, nnc *v1alpha.NodeNetworkConfig) cnstypes.ResponseCode
}

// TODO(rbtr) where should this live??
// reconcileInitialCNSState initializes cns by passing pods and a CreateNetworkContainerRequest
func reconcileInitialCNSState(ctx context.Context, cli nodeNetworkConfigGetter, ncReconciler ncStateReconciler, podInfoByIPProvider cns.PodInfoByIPProvider) error {
	// Get nnc using direct client
	nnc, err := cli.Get(ctx)
	if err != nil {

		if crd.IsNotDefined(err) {
			return errors.Wrap(err, "failed to get NNC during init CNS state")
		}

		// If instance of crd is not found, pass nil to CNSClient
		if client.IgnoreNotFound(err) == nil {
			err = restserver.ResponseCodeToError(ncReconciler.ReconcileNCState(nil, nil, nnc))
			return errors.Wrap(err, "failed to reconcile NC state")
		}

		// If it's any other error, log it and return
		return errors.Wrap(err, "error getting NodeNetworkConfig when initializing CNS state")
	}

	// If there are no NCs, pass nil to CNSClient
	if len(nnc.Status.NetworkContainers) == 0 {
		err = restserver.ResponseCodeToError(ncReconciler.ReconcileNCState(nil, nil, nnc))
		return errors.Wrap(err, "failed to reconcile NC state")
	}

	// Convert to CreateNetworkContainerRequest
	for i := range nnc.Status.NetworkContainers {
		var ncRequest *cns.CreateNetworkContainerRequest
		var err error

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

		// rebuild CNS state
		podInfoByIP, err := podInfoByIPProvider.PodInfoByIP()
		if err != nil {
			return errors.Wrap(err, "provider failed to provide PodInfoByIP")
		}

		// Call cnsclient init cns passing those two things.
		if err := restserver.ResponseCodeToError(ncReconciler.ReconcileNCState(ncRequest, podInfoByIP, nnc)); err != nil {
			return errors.Wrap(err, "failed to reconcile NC state")
		}
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
			return errors.Wrap(err, "failed to create CNS PodInfoProvider")
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
	nnccli, err := nodenetworkconfig.NewClient(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create NNC client")
	}
	// TODO(rbtr): nodename and namespace should be in the cns config
	scopedcli := nncctrl.NewScopedClient(nnccli, types.NamespacedName{Namespace: "kube-system", Name: nodeName})

	clusterSubnetStateChan := make(chan v1alpha1.ClusterSubnetState)
	// initialize the ipam pool monitor
	poolOpts := ipampool.Options{
		RefreshDelay: poolIPAMRefreshRateInMilliseconds * time.Millisecond,
	}
	poolMonitor := ipampool.NewMonitor(httpRestServiceImplementation, scopedcli, clusterSubnetStateChan, &poolOpts)
	httpRestServiceImplementation.IPAMPoolMonitor = poolMonitor

	// reconcile initial CNS state from CNI or apiserver.
	// Only reconcile if there are any existing Pods using NC ips,
	// else let the goal state be updated using a regular NNC Reconciler loop
	podInfoByIP, err := podInfoByIPProvider.PodInfoByIP()
	if err != nil {
		return errors.Wrap(err, "failed to provide PodInfoByIP")
	}
	if len(podInfoByIP) > 0 {
		logger.Printf("Reconciling initial CNS state as PodInfoByIP is not empty: %d", len(podInfoByIP))

		// apiserver nnc might not be registered or api server might be down and crashloop backof puts us outside of 5-10 minutes we have for
		// aks addons to come up so retry a bit more aggresively here.
		// will retry 10 times maxing out at a minute taking about 8 minutes before it gives up.
		attempt := 0
		err = retry.Do(func() error {
			attempt++
			logger.Printf("reconciling initial CNS state attempt: %d", attempt)
			err = reconcileInitialCNSState(ctx, scopedcli, httpRestServiceImplementation, podInfoByIPProvider)
			if err != nil {
				logger.Errorf("failed to reconcile initial CNS state, attempt: %d err: %v", attempt, err)
			}
			return errors.Wrap(err, "failed to initialize CNS state")
		}, retry.Context(ctx), retry.Delay(initCNSInitalDelay), retry.MaxDelay(time.Minute))
		if err != nil {
			return err
		}
		logger.Printf("reconciled initial CNS state after %d attempts", attempt)
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

	// the nodeScopedCache sets Selector options on the Manager cache which are used
	// to perform *server-side* filtering of the cached objects. This is very important
	// for high node/pod count clusters, as it keeps us from watching objects at the
	// whole cluster scope when we are only interested in the Node's scope.
	nodeScopedCache := cache.BuilderWithOptions(cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&v1alpha.NodeNetworkConfig{}: {
				Field: fields.SelectorFromSet(fields.Set{"metadata.name": nodeName}),
			},
		},
	})

	manager, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme:             nodenetworkconfig.Scheme,
		MetricsBindAddress: "0",
		Namespace:          "kube-system", // TODO(rbtr): namespace should be in the cns config
		NewCache:           nodeScopedCache,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create manager")
	}

	// get our Node so that we can xref it against the NodeNetworkConfig's to make sure that the
	// NNC is not stale and represents the Node we're running on.
	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get node %s", nodeName)
	}

	// get CNS Node IP to compare NC Node IP with this Node IP to ensure NCs were created for this node
	nodeIP := configuration.NodeIP()

	// NodeNetworkConfig reconciler
	nncReconciler := nncctrl.NewReconciler(httpRestServiceImplementation, nnccli, poolMonitor, nodeIP)
	// pass Node to the Reconciler for Controller xref
	if err := nncReconciler.SetupWithManager(manager, node); err != nil { //nolint:govet // intentional shadow
		return errors.Wrapf(err, "failed to setup nnc reconciler with manager")
	}

	if cnsconfig.EnableSubnetScarcity {
		cssCli, err := clustersubnetstate.NewClient(kubeConfig)
		if err != nil {
			return errors.Wrapf(err, "failed to init css client")
		}

		// ClusterSubnetState reconciler
		cssReconciler := cssctrl.Reconciler{
			Cli:  cssCli,
			Sink: clusterSubnetStateChan,
		}
		if err := cssReconciler.SetupWithManager(manager); err != nil {
			return errors.Wrapf(err, "failed to setup css reconciler with manager")
		}
	}

	// adding some routes to the root service mux
	mux := httpRestServiceImplementation.Listener.GetMux()
	mux.Handle("/readyz", http.StripPrefix("/readyz", &healthz.Handler{}))
	if cnsconfig.EnablePprof {
		// add pprof endpoints
		mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
		mux.Handle("/debug/pprof/block", pprof.Handler("block"))
		mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	// Start the Manager which starts the reconcile loop.
	// The Reconciler will send an initial NodeNetworkConfig update to the PoolMonitor, starting the
	// Monitor's internal loop.
	go func() {
		logger.Printf("Starting NodeNetworkConfig reconciler.")
		for {
			if err := manager.Start(ctx); err != nil {
				logger.Errorf("[Azure CNS] Failed to start request controller: %v", err)
				// retry to start the request controller
				// todo: add a CNS metric to count # of failures
			} else {
				logger.Printf("exiting NodeNetworkConfig reconciler")
				return
			}

			// Retry after 1sec
			time.Sleep(time.Second)
		}
	}()
	logger.Printf("initialized NodeNetworkConfig reconciler")
	// wait for the Reconciler to run once on a NNC that was made for this Node
	if started := nncReconciler.Started(ctx); !started {
		return errors.Errorf("context cancelled while waiting for reconciler start")
	}
	logger.Printf("started NodeNetworkConfig reconciler")

	go func() {
		logger.Printf("starting SyncHostNCVersion loop")
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(time.Duration(cnsconfig.SyncHostNCVersionIntervalMs) * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				timedCtx, cancel := context.WithTimeout(ctx, time.Duration(cnsconfig.SyncHostNCVersionIntervalMs)*time.Millisecond)
				httpRestServiceImplementation.SyncHostNCVersion(timedCtx, cnsconfig.ChannelMode)
				cancel()
			case <-ctx.Done():
				logger.Printf("exiting SyncHostNCVersion")
				return
			}
		}
	}()
	logger.Printf("initialized and started SyncHostNCVersion loop")

	return nil
}
