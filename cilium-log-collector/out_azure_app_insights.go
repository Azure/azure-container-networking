package main

import "C" //nolint

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unsafe" //nolint

	"github.com/Azure/azure-container-networking/common"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
)

const (
	// disableFilePath is the path where the disable configmap is mounted
	disableFilePath = "/fluent-bit/etc/disable/disable-cilium-log-collector"
)

// version is set at build time
var version = ""

// RecordProcessor handles batch record processing for testability
type RecordProcessor struct {
	tracker  AppInsightsTracker
	tag      string
	debug    bool
	logKey   string
	disabled bool
	version  string
}

// ProcessRecord represents a single log record
type ProcessRecord struct {
	Timestamp time.Time
	Fields    map[interface{}]interface{}
}

// AppInsightsTracker abstracts telemetry tracking for testing
type AppInsightsTracker interface {
	Track(telemetry appinsights.Telemetry)
}

// RealAppInsightsTracker wraps the actual App Insights client
type RealAppInsightsTracker struct {
	client appinsights.TelemetryClient
}

func (r *RealAppInsightsTracker) Track(telemetry appinsights.Telemetry) {
	r.client.Track(telemetry)
}

var (
	client       appinsights.TelemetryClient
	debug        string
	logKey       string
	hostMetadata *common.Metadata
	disabled     bool
)

func convertToString(v interface{}) string {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case string:
		return val
	case map[interface{}]interface{}:
		converted := convertToJSONCompatible(val)
		if jsonBytes, err := json.Marshal(converted); err == nil {
			return string(jsonBytes)
		}
		return fmt.Sprintf("%+v", converted)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// convertToJSONCompatible recursively converts map[interface{}]interface{} to map[string]interface{}
// json.Marshal only works with map[string]interface{}
func convertToJSONCompatible(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		converted := make(map[string]interface{})
		for k, v := range val {
			keyStr := fmt.Sprintf("%v", k)
			converted[keyStr] = convertToJSONCompatible(v)
		}
		return converted
	case []byte:
		// returning %v will lead to byte arrays, but we want string values
		return string(val)
	default:
		// returning v directly leads to base64 values, so convert to string first
		return fmt.Sprintf("%v", val)
	}
}

//export FLBPluginRegister
func FLBPluginRegister(def unsafe.Pointer) int {
	return output.FLBPluginRegister(def, "azure_app_insights", "Azure application insights")
}

// (fluentbit will call this)
// plugin (context) pointer to fluentbit context (state/ c code)
//
//export FLBPluginInit
func FLBPluginInit(plugin unsafe.Pointer) int {
	fmt.Printf("[flb-azure-app-insights] version = '%s'\n", version)
	// check disable flag
	if _, err := os.Stat(disableFilePath); err == nil {
		fmt.Printf("[flb-azure-app-insights] Plugin disabled- file found at: %s\n", disableFilePath)
		disabled = true
		return output.FLB_OK
	}
	disabled = false

	instrumentationKey := output.FLBPluginConfigKey(plugin, "instrumentation_key")
	// the key that is identified as the log upon receiving the record in this plugin
	logKey = output.FLBPluginConfigKey(plugin, "log_key")
	if logKey == "" {
		logKey = "log"
	}
	debug = output.FLBPluginConfigKey(plugin, "debug")
	imds := output.FLBPluginConfigKey(plugin, "imds")
	fmt.Printf("[flb-azure-app-insights] plugin instrumentation key = '%s'\n", instrumentationKey)
	fmt.Printf("[flb-azure-app-insights] using log key = '%s'\n", logKey)
	fmt.Printf("[flb-azure-app-insights] debug = '%s'\n", debug)
	fmt.Printf("[flb-azure-app-insights] imds = '%s'\n", imds)

	telemetryConfig := appinsights.NewTelemetryConfiguration(instrumentationKey)
	// max time to wait before sending a batch of telemetry
	telemetryConfig.MaxBatchInterval = 10 * time.Second
	// max number of telemetry items in each request
	telemetryConfig.MaxBatchSize = 10
	client = appinsights.NewTelemetryClientFromConfig(telemetryConfig)

	// retrieve IMDS data once
	if imds == "true" {
		metadata, err := common.GetHostMetadata("/tmp/metadata.json")
		if err != nil {
			fmt.Printf("[flb-azure-app-insights] Warning: Failed to get IMDS metadata: %v\n", err)
		} else {
			fmt.Print("[flb-azure-app-insights] Retrieved IMDS metadata\n")
			hostMetadata = &metadata
		}
	}

	fmt.Printf("[flb-azure-app-insights] App Insights client initialized with key: %s\n",
		telemetryConfig.InstrumentationKey)
	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}

	dec := output.NewDecoder(data, int(length))
	tracker := &RealAppInsightsTracker{client: client}
	processor := &RecordProcessor{
		tracker:  tracker,
		tag:      C.GoString(tag),
		debug:    debug == "true",
		logKey:   logKey,
		disabled: disabled,
		version:  version,
	}

	count := 0
	for {
		ret, ts, record = output.GetRecord(dec)
		if ret != 0 {
			break
		}

		var timestamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timestamp = ts.(output.FLBTime).Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			fmt.Println("time provided invalid, defaulting to now.")
			timestamp = time.Now()
		}

		processor.ProcessSingleRecord(ProcessRecord{
			Timestamp: timestamp,
			Fields:    record,
		}, count, hostMetadata)
		count++
	}

	return output.FLB_OK
}

// ProcessSingleRecord handles processing of an individual record
func (rp *RecordProcessor) ProcessSingleRecord(record ProcessRecord, recordIndex int, metadata *common.Metadata) {
	// if disabled, skip processing
	if rp.disabled {
		return
	}

	customFields := make(map[string]string)
	var logMessage string

	for k, v := range record.Fields {
		keyStr := convertToString(k)
		valueStr := convertToString(v)

		if keyStr == rp.logKey {
			logMessage = valueStr
		} else {
			customFields[keyStr] = valueStr
		}
	}
	customFields["fluentbit_tag"] = rp.tag
	customFields["record_count"] = strconv.Itoa(recordIndex)
	customFields["cilium_log_collector_version"] = rp.version

	if metadata != nil {
		customFields["azure_location"] = metadata.Location
		customFields["azure_vm_name"] = metadata.VMName
		customFields["azure_offer"] = metadata.Offer
		customFields["azure_os_type"] = metadata.OsType
		customFields["azure_placement_group_id"] = metadata.PlacementGroupID
		customFields["azure_platform_fault_domain"] = metadata.PlatformFaultDomain
		customFields["azure_platform_update_domain"] = metadata.PlatformUpdateDomain
		customFields["azure_publisher"] = metadata.Publisher
		customFields["azure_resource_group_name"] = metadata.ResourceGroupName
		customFields["azure_sku"] = metadata.Sku
		customFields["azure_subscription_id"] = metadata.SubscriptionID
		customFields["azure_tags"] = metadata.Tags
		customFields["azure_os_version"] = metadata.OSVersion
		customFields["azure_vm_id"] = metadata.VMID
		customFields["azure_vm_size"] = metadata.VMSize
		customFields["azure_kernel_version"] = metadata.KernelVersion
	}

	if rp.debug {
		var msgBuilder strings.Builder
		msgBuilder.WriteString(fmt.Sprintf("[flb-azure-app-insights] #%d %s: [%s, {", recordIndex, rp.tag,
			record.Timestamp.String()))
		for k, v := range customFields {
			msgBuilder.WriteString(fmt.Sprintf("\"%s\": %s, ", k, v))
		}
		msgBuilder.WriteString("}\n")
		fmt.Print(msgBuilder.String())
		fmt.Printf("[flb-azure-app-insights] Sent trace to App Insights: log msg=%d chars, %d custom fields\n", len(logMessage), len(customFields))
	}

	trace := appinsights.NewTraceTelemetry(logMessage, appinsights.Information)
	for key, value := range customFields {
		trace.Properties[key] = value
	}
	rp.tracker.Track(trace)
}

//export FLBPluginExit
func FLBPluginExit() int {
	if client != nil {
		client.Channel().Flush()
		time.Sleep(2 * time.Second)
		fmt.Println("[flb-azure-app-insights] App Insights client flushed and closed")
	}
	return output.FLB_OK
}

func main() {
}
