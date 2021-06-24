// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	cni "github.com/Azure/azure-container-networking/cns/cnireconciler"
	"github.com/Azure/azure-container-networking/cns/cnsclient"
	"github.com/Azure/azure-container-networking/cns/common"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/imdsclient"
	"github.com/Azure/azure-container-networking/cns/ipampoolmonitor"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller"
	"github.com/Azure/azure-container-networking/cns/multitenantcontroller/multitenantoperator"
	"github.com/Azure/azure-container-networking/cns/nmagentclient"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/Azure/azure-container-networking/cns/singletenantcontroller"
	"github.com/Azure/azure-container-networking/cns/singletenantcontroller/kubecontroller"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	localtls "github.com/Azure/azure-container-networking/server/tls"
	"github.com/Azure/azure-container-networking/store"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	// Service name.
	name                              = "azure-cns"
	pluginName                        = "azure-vnet"
	defaultCNINetworkConfigFileName   = "10-azure.conflist"
	configFileName                    = "config.json"
	dncApiVersion                     = "?api-version=2018-03-01"
	poolIPAMRefreshRateInMilliseconds = 1000

	// 720 * acn.FiveSeconds sec sleeps = 1Hr
	maxRetryNodeRegister = 720
)

var rootCtx context.Context
var rootErrCh chan error

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
		Description:  "Debug flag to retrieve IPconfigs, available values: allocated, available, all",
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

	nodeRegisterRequest.NumCPU = numCPU
	supportedApis, retErr := nmagentclient.GetNmAgentSupportedApis(httpc, "")

	if retErr != nil {
		logger.Errorf("[Azure CNS] Failed to retrieve SupportedApis from NMagent of node %s with Infrastructure Network: %s PrivateEndpoint: %s",
			nodeID, infraVnet, dncEP)
		return retErr
	}

	//To avoid any null-pointer deferencing errors.
	if supportedApis == nil {
		supportedApis = []string{}
	}

	nodeRegisterRequest.NmAgentSupportedApis = supportedApis

	//CNS tries to register Node for maximum of an hour.
	for tryNum := 0; tryNum <= maxRetryNodeRegister; tryNum++ {
		success, err := sendRegisterNodeRequest(httpc, httpRestService, nodeRegisterRequest, url)
		if err != nil {
			return err
		}
		if success {
			return nil
		}
		time.Sleep(acn.FiveSeconds)
	}
	return fmt.Errorf("[Azure CNS] Failed to register node %s after maximum reties for an hour with Infrastructure Network: %s PrivateEndpoint: %s",
		nodeID, infraVnet, dncEP)
}

// sendRegisterNodeRequest func helps in registering the node until there is an error.
func sendRegisterNodeRequest(
	httpc *http.Client,
	httpRestService cns.HTTPService,
	nodeRegisterRequest cns.NodeRegisterRequest,
	registerURL string) (bool, error) {

	var (
		body     bytes.Buffer
		response *http.Response
		err      = fmt.Errorf("")
	)

	err = json.NewEncoder(&body).Encode(nodeRegisterRequest)
	if err != nil {
		log.Errorf("[Azure CNS] Failed to register node while encoding json failed with non-retriable err %v", err)
		return false, err
	}

	response, err = httpc.Post(registerURL, "application/json", &body)
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to register node with retriable err: %+v", err)
		return false, nil
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusCreated {
		err = fmt.Errorf("[Azure CNS] Failed to register node, DNC replied with http status code %s", strconv.Itoa(response.StatusCode))
		logger.Errorf(err.Error())
		return false, nil
	}

	var req cns.SetOrchestratorTypeRequest
	err = json.NewDecoder(response.Body).Decode(&req)
	if err != nil {
		log.Errorf("[Azure CNS] decoding Node Resgister response json failed with err %v", err)
		return false, nil
	}
	httpRestService.SetNodeOrchestrator(&req)

	logger.Printf("[Azure CNS] Node Registered")
	return true, nil
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

	if vers {
		printVersion()
		os.Exit(0)
	}

	// Initialize CNS.
	var (
		err    error
		config common.ServiceConfig
	)

	config.Version = version
	config.Name = name
	// Create a channel to receive unhandled errors from CNS.
	config.ErrChan = rootErrCh

	// Create logging provider.
	logger.InitLogger(name, logLevel, logTarget, logDirectory)

	if clientDebugCmd != "" {
		err := cnsclient.HandleCNSClientCommands(clientDebugCmd, clientDebugArg)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if !telemetryEnabled {
		logger.Errorf("[Azure CNS] Cannot disable telemetry via cmdline. Update cns_config.json to disable telemetry.")
	}

	cnsconfig, err := configuration.ReadConfig()
	if err != nil {
		logger.Errorf("[Azure CNS] Error reading cns config: %v", err)
	}

	configuration.SetCNSConfigDefaults(&cnsconfig)
	logger.Printf("[Azure CNS] Read config :%+v", cnsconfig)

	if cnsconfig.WireserverIP != "" {
		nmagentclient.WireserverIP = cnsconfig.WireserverIP
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

	// Log platform information.
	logger.Printf("Running on %v", platform.GetOSInfo())

	err = platform.CreateDirectory(storeFileLocation)
	if err != nil {
		logger.Errorf("Failed to create File Store directory %s, due to Error:%v", storeFileLocation, err.Error())
		return
	}

	// Create the key value store.
	storeFileName := storeFileLocation + name + ".json"
	config.Store, err = store.NewJsonFileStore(storeFileName)
	if err != nil {
		logger.Errorf("Failed to create store file: %s, due to error %v\n", storeFileName, err)
		return
	}

	nmaclient, err := nmagentclient.NewNMAgentClient("")
	if err != nil {
		logger.Errorf("Failed to start nmagent client due to error %v", err)
		return
	}
	// Create CNS object.
	httpRestService, err := restserver.NewHTTPRestService(&config, new(imdsclient.ImdsClient), nmaclient)
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
				TLSSubjectName:     cnsconfig.TLSSubjectName,
				TLSCertificatePath: cnsconfig.TLSCertificatePath,
				TLSPort:            cnsconfig.TLSPort,
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
		// We might be configured to reinitialize state from the CNI instead of the apiserver.
		// If so, we should check that the the CNI is new enough to support the state commands,
		// otherwise we fall back to the existing behavior.
		if cnsconfig.InitializeFromCNI {
			isGoodVer, err := cni.IsDumpStateVer()
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
		if cnsconfig.InitializeFromCNI {
			logger.Printf("Initializing from CNI")
		} else {
			logger.Printf("Initializing from Kubernetes")
		}
		logger.Printf("Set GlobalPodInfoScheme %v", cns.GlobalPodInfoScheme)

		err = InitializeCRDState(httpRestService, cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start CRD Controller, err:%v.\n", err)
			return
		}
	}

	// Initialize multi-tenant controller if the CNS is running in MultiTenantCRD mode.
	// It must be started before we start HTTPRestService.
	if config.ChannelMode == cns.MultiTenantCRD {
		err = InitializeMultiTenantController(httpRestService, cnsconfig)
		if err != nil {
			logger.Errorf("Failed to start multiTenantController, err:%v.\n", err)
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
				select {
				case <-tickerChannel:
					httpRestService.SyncNodeStatus(ep, vnet, node, json.RawMessage{})
				}
			}
		}(privateEndpoint, infravnet, nodeID)
	}

	var netPlugin network.NetPlugin
	var ipamPlugin ipam.IpamPlugin

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

		// Create the key value store.
		pluginStoreFile := storeFileLocation + pluginName + ".json"
		pluginConfig.Store, err = store.NewJsonFileStore(pluginStoreFile)
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
	}

	logger.Printf("CNS exited")
	logger.Close()
}

func InitializeMultiTenantController(httpRestService cns.HTTPService, cnsconfig configuration.CNSConfig) error {
	var multiTenantController multitenantcontroller.RequestController
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return err
	}

	//convert interface type to implementation type
	httpRestServiceImpl, ok := httpRestService.(*restserver.HTTPRestService)
	if !ok {
		logger.Errorf("Failed to convert interface httpRestService to implementation: %v", httpRestService)
		return fmt.Errorf("Failed to convert interface httpRestService to implementation: %v",
			httpRestService)
	}

	// Set orchestrator type
	orchestrator := cns.SetOrchestratorTypeRequest{
		OrchestratorType: cns.KubernetesCRD,
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
			if err := multiTenantController.Start(rootCtx); err != nil {
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
	rootCxt := context.Background()
	go func() {
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(cnsconfig.SyncHostNCVersionIntervalMs * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				httpRestServiceImpl.SyncHostNCVersion(rootCxt, cnsconfig.ChannelMode, cnsconfig.SyncHostNCTimeoutMs)
			case <-rootCxt.Done():
				return
			}
		}
	}()

	return nil
}

// initializeCRD state
func InitializeCRDState(httpRestService cns.HTTPService, cnsconfig configuration.CNSConfig) error {
	var requestController singletenantcontroller.RequestController

	logger.Printf("[Azure CNS] Starting request controller")

	kubeConfig, err := kubecontroller.GetKubeConfig()
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to get kubeconfig for request controller: %v", err)
		return err
	}

	//convert interface type to implementation type
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

	// Get crd implementation of request controller
	requestController, err = kubecontroller.New(
		kubecontroller.Config{
			InitializeFromCNI: cnsconfig.InitializeFromCNI,
			KubeConfig:        kubeConfig,
			Service:           httpRestServiceImplementation,
		})
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to make crd request controller :%v", err)
		return err
	}

	// initialize the ipam pool monitor
	httpRestServiceImplementation.IPAMPoolMonitor = ipampoolmonitor.NewCNSIPAMPoolMonitor(httpRestServiceImplementation, requestController)

	err = requestController.Init(rootCtx)
	if err != nil {
		logger.Errorf("[Azure CNS] Failed to initialized cns state :%v", err)
		return err
	}

	//Start the RequestController which starts the reconcile loop
	go func() {
		for {
			if err := requestController.Start(rootCtx); err != nil {
				logger.Errorf("[Azure CNS] Failed to start request controller: %v", err)
				// retry to start the request controller
				// todo: add a CNS metric to count # of failures
			} else {
				logger.Printf("[Azure CNS] Exiting RequestController")
				return
			}

			// Retry after 1sec
			time.Sleep(time.Second)
		}
	}()

	for {
		if requestController.IsStarted() {
			logger.Printf("RequestController is started")
			break
		}

		logger.Printf("Waiting for requestController to start...")
		time.Sleep(time.Millisecond * 500)
	}

	logger.Printf("Starting IPAM Pool Monitor")
	go func() {
		for {
			if err := httpRestServiceImplementation.IPAMPoolMonitor.Start(rootCtx, poolIPAMRefreshRateInMilliseconds); err != nil {
				logger.Errorf("[Azure CNS] Failed to start pool monitor with err: %v", err)
				// todo: add a CNS metric to count # of failures
			} else {
				logger.Printf("[Azure CNS] Exiting IPAM Pool Monitor")
				return
			}

			// Retry after 1sec
			time.Sleep(time.Second)
		}
	}()

	logger.Printf("Starting SyncHostNCVersion")
	go func() {
		// Periodically poll vfp programmed NC version from NMAgent
		tickerChannel := time.Tick(cnsconfig.SyncHostNCVersionIntervalMs * time.Millisecond)
		for {
			select {
			case <-tickerChannel:
				httpRestServiceImplementation.SyncHostNCVersion(rootCtx, cnsconfig.ChannelMode, cnsconfig.SyncHostNCTimeoutMs)
			case <-rootCtx.Done():
				return
			}
		}
	}()

	return nil
}
