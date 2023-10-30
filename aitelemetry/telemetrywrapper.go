package aitelemetry

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
)

const (
	resourceGroupStr                 = "ResourceGroup"
	vmSizeStr                        = "VMSize"
	osVersionStr                     = "OSVersion"
	osStr                            = "OS"
	locationStr                      = "Region"
	appNameStr                       = "AppName"
	subscriptionIDStr                = "SubscriptionID"
	vmNameStr                        = "VMName"
	vmIDStr                          = "VMID"
	versionStr                       = "AppVersion"
	azurePublicCloudStr              = "AzurePublicCloud"
	hostNameKey                      = "hostname"
	defaultTimeout                   = 10
	defaultBatchIntervalInSecs       = 15
	defaultBatchSizeInBytes          = 32768
	defaultGetEnvRetryCount          = 5
	defaultGetEnvRetryWaitTimeInSecs = 3
	defaultRefreshTimeoutInSecs      = 10
)

var debugMode bool

func setAIConfigDefaults(config *AIConfig) {
	if config.RefreshTimeout == 0 {
		config.RefreshTimeout = defaultRefreshTimeoutInSecs
	}

	if config.BatchInterval == 0 {
		config.BatchInterval = defaultBatchIntervalInSecs
	}

	if config.BatchSize == 0 {
		config.BatchSize = defaultBatchSizeInBytes
	}

	if config.GetEnvRetryCount == 0 {
		config.GetEnvRetryCount = defaultGetEnvRetryCount
	}

	if config.GetEnvRetryWaitTimeInSecs == 0 {
		config.GetEnvRetryWaitTimeInSecs = defaultGetEnvRetryWaitTimeInSecs
	}
}

func messageListener() appinsights.DiagnosticsMessageListener {
	if debugMode {
		return appinsights.NewDiagnosticsMessageListener(func(msg string) error {
			debugLog("[AppInsights] [%s] %s\n", time.Now().Format(time.UnixDate), msg)
			return nil
		})
	}

	return nil
}

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf(format, args...)
	}
}

func getMetadata(th *telemetryHandle) {
	var metadata common.Metadata
	var err error

	if th.refreshTimeout < 4 {
		th.refreshTimeout = defaultTimeout
	}

	// check if metadata in memory otherwise initiate wireserver request
	for {
		metadata, err = common.GetHostMetadata(metadataFile)
		if err == nil || th.disableMetadataRefreshThread {
			break
		}

		debugLog("[AppInsights] Error getting metadata %v. Sleep for %d", err, th.refreshTimeout)
		time.Sleep(time.Duration(th.refreshTimeout) * time.Second)
	}

	if err != nil {
		debugLog("[AppInsights] Error getting metadata %v", err)
		return
	}

	// acquire write lock before writing metadata to telemetry handle
	th.rwmutex.Lock()
	th.metadata = metadata
	th.rwmutex.Unlock()

	lockclient, err := processlock.NewFileLock(metadataFile + store.LockExtension)
	if err != nil {
		log.Printf("Error initializing file lock:%v", err)
		return
	}

	// Save metadata retrieved from wireserver to a file
	kvs, err := store.NewJsonFileStore(metadataFile, lockclient, nil)
	if err != nil {
		debugLog("[AppInsights] Error initializing kvs store: %v", err)
		return
	}
	// Acquire store lock.
	if err = kvs.Lock(store.DefaultLockTimeout); err != nil {
		log.Errorf("getMetadata: Not able to acquire lock:%v", err)
		return
	}
	metadataErr := common.SaveHostMetadata(th.metadata, metadataFile)
	err = kvs.Unlock()
	if err != nil {
		log.Errorf("getMetadata: Not able to release lock:%v", err)
	}

	if metadataErr != nil {
		debugLog("[AppInsights] saving host metadata failed with :%v", err)
	}
}

func isPublicEnvironment(url string, retryCount, waitTimeInSecs int) (bool, error) {
	var (
		cloudName string
		err       error
	)

	for i := 0; i < retryCount; i++ {
		cloudName, err = common.GetAzureCloud(url)
		if cloudName == azurePublicCloudStr {
			debugLog("[AppInsights] CloudName: %s\n", cloudName)
			return true, nil
		} else if err == nil {
			debugLog("[AppInsights] This is not azure public cloud:%s", cloudName)
			return false, fmt.Errorf("Not an azure public cloud: %s", cloudName)
		}

		debugLog("GetAzureCloud returned err :%v", err)
		time.Sleep(time.Duration(waitTimeInSecs) * time.Second)
	}

	return false, err
}

// NewAITelemetry creates telemetry handle with user specified appinsights id.
func NewAITelemetry(
	azEnvUrl string,
	id string,
	aiConfig AIConfig,
) (TelemetryHandle, error) {
	debugMode = aiConfig.DebugMode

	if id == "" {
		debugLog("Empty AI key")
		return nil, fmt.Errorf("AI key is empty")
	}

	setAIConfigDefaults(&aiConfig)

	// check if azure instance is in public cloud
	isPublic, err := isPublicEnvironment(azEnvUrl, aiConfig.GetEnvRetryCount, aiConfig.GetEnvRetryWaitTimeInSecs)
	if !isPublic {
		return nil, err
	}

	telemetryConfig := appinsights.NewTelemetryConfiguration(id)
	telemetryConfig.MaxBatchSize = aiConfig.BatchSize
	telemetryConfig.MaxBatchInterval = time.Duration(aiConfig.BatchInterval) * time.Second

	th := &telemetryHandle{
		client:                       appinsights.NewTelemetryClientFromConfig(telemetryConfig),
		appName:                      aiConfig.AppName,
		appVersion:                   aiConfig.AppVersion,
		diagListener:                 messageListener(),
		disableMetadataRefreshThread: aiConfig.DisableMetadataRefreshThread,
		refreshTimeout:               aiConfig.RefreshTimeout,
	}

	if th.disableMetadataRefreshThread {
		getMetadata(th)
	} else {
		go getMetadata(th)
	}

	return th, nil
}

// TrackLog function sends report (trace) to appinsights resource. It overrides few of the existing columns with app information
// and for rest it uses custom dimesion
func (th *telemetryHandle) TrackLog(report Report) {
	// Initialize new trace message
	trace := appinsights.NewTraceTelemetry(report.Message, appinsights.Warning)

	// will be empty if cns used as telemetry service for cni
	if th.appVersion == "" {
		th.appVersion = report.AppVersion
	}

	// Override few of existing columns with metadata
	trace.Tags.User().SetAuthUserId(runtime.GOOS)
	trace.Tags.Operation().SetId(report.Context)
	trace.Tags.Operation().SetParentId(th.appVersion)
	trace.Tags.Application().SetVer(th.appVersion)
	trace.Properties[hostNameKey], _ = os.Hostname()

	// copy app specified custom dimension
	for key, value := range report.CustomDimensions {
		trace.Properties[key] = value
	}

	trace.Properties[appNameStr] = th.appName

	// Acquire read lock to read metadata
	th.rwmutex.RLock()
	metadata := th.metadata
	th.rwmutex.RUnlock()

	// Check if metadata is populated
	if metadata.SubscriptionID != "" {
		// copy metadata from wireserver to trace
		trace.Tags.User().SetAccountId(metadata.SubscriptionID)
		trace.Tags.User().SetId(metadata.VMName)
		trace.Properties[locationStr] = metadata.Location
		trace.Properties[resourceGroupStr] = metadata.ResourceGroupName
		trace.Properties[vmSizeStr] = metadata.VMSize
		trace.Properties[osVersionStr] = metadata.OSVersion
		trace.Properties[vmIDStr] = metadata.VMID
		trace.Tags.Session().SetId(metadata.VMID)
	}

	// send to appinsights resource
	th.client.Track(trace)
}

// TrackEvent function sends events to appinsights resource. It overrides a few of the existing columns
// with app information.
func (th *telemetryHandle) TrackEvent(event Event) {
	// Initialize new event message
	aiEvent := appinsights.NewEventTelemetry(event.EventName)
	// OperationId => resourceID (e.g.: NCID)
	aiEvent.Tags.Operation().SetId(event.ResourceID)

	// Copy the properties, if supplied
	if event.Properties != nil {
		for key, value := range event.Properties {
			aiEvent.Properties[key] = value
		}
	}

	// Acquire read lock to read metadata
	th.rwmutex.RLock()
	metadata := th.metadata
	th.rwmutex.RUnlock()

	// Add metadata
	if metadata.SubscriptionID != "" {
		aiEvent.Tags.User().SetAccountId(metadata.SubscriptionID)
		// AnonId => VMName
		aiEvent.Tags.User().SetId(metadata.VMName)
		// SessionId => VMID
		aiEvent.Tags.Session().SetId(metadata.VMID)
		aiEvent.Properties[locationStr] = metadata.Location
		aiEvent.Properties[resourceGroupStr] = metadata.ResourceGroupName
		aiEvent.Properties[vmSizeStr] = metadata.VMSize
		aiEvent.Properties[osVersionStr] = metadata.OSVersion
		aiEvent.Properties[vmIDStr] = metadata.VMID
		aiEvent.Properties[vmNameStr] = metadata.VMName
	}

	aiEvent.Tags.Operation().SetParentId(th.appVersion)
	aiEvent.Tags.User().SetAuthUserId(runtime.GOOS)
	aiEvent.Properties[osStr] = runtime.GOOS
	aiEvent.Properties[appNameStr] = th.appName
	aiEvent.Properties[versionStr] = th.appVersion
	th.client.Track(aiEvent)
}

// TrackMetric function sends metric to appinsights resource. It overrides few of the existing columns with app information
// and for rest it uses custom dimesion
func (th *telemetryHandle) TrackMetric(metric Metric) {
	// Initialize new metric
	aimetric := appinsights.NewMetricTelemetry(metric.Name, metric.Value)

	// Acquire read lock to read metadata
	th.rwmutex.RLock()
	metadata := th.metadata
	th.rwmutex.RUnlock()

	if th.appVersion == "" {
		th.appVersion = metric.AppVersion
	}

	// Check if metadata is populated
	if metadata.SubscriptionID != "" {
		aimetric.Properties[locationStr] = metadata.Location
		aimetric.Properties[subscriptionIDStr] = metadata.SubscriptionID
		aimetric.Properties[vmNameStr] = metadata.VMName
		aimetric.Properties[versionStr] = th.appVersion
		aimetric.Properties[resourceGroupStr] = th.metadata.ResourceGroupName
		aimetric.Properties[vmIDStr] = metadata.VMID
		aimetric.Properties[osStr] = runtime.GOOS
		aimetric.Tags.Session().SetId(metadata.VMID)
	}

	// copy custom dimensions
	for key, value := range metric.CustomDimensions {
		aimetric.Properties[key] = value
	}

	// send metric to appinsights
	th.client.Track(aimetric)
}

// Close - should be called for each NewAITelemetry call. Will release resources acquired
func (th *telemetryHandle) Close(timeout int) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	// wait for items to be sent otherwise timeout
	<-th.client.Channel().Close(time.Duration(timeout) * time.Second)

	// Remove diganostic message listener
	if th.diagListener != nil {
		th.diagListener.Remove()
		th.diagListener = nil
	}
}

// Flush - forces the current queue to be sent
func (th *telemetryHandle) Flush() {
	th.client.Channel().Flush()
}
