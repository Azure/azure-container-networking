package aitelemetry

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

var (
	th               TelemetryHandle
	hostAgentUrl     = "localhost:3501"
	getCloudResponse = "AzurePublicCloud"
	httpURL          = "http://" + hostAgentUrl
	// connectionString = "InstrumentationKey=00000000-0000-0000-0000-000000000000;IngestionEndpoint=https://ingestion.endpoint.com/;LiveEndpoint=https://live.endpoint.com/;ApplicationId=11111111-1111-1111-1111-111111111111"
)

func TestMain(m *testing.M) {
	log.SetName("testaitelemetry")
	log.SetLevel(log.LevelInfo)
	err := log.SetTargetLogDirectory(log.TargetLogfile, "/var/log/")
	if err == nil {
		fmt.Printf("TestST LogDir configuration succeeded\n")
	}

	p := platform.NewExecClient(nil)
	if runtime.GOOS == "linux" {
		//nolint:errcheck // initial test setup
		p.ExecuteRawCommand("cp metadata_test.json /tmp/azuremetadata.json")
	} else {
		metadataFile := filepath.FromSlash(os.Getenv("TEMP")) + "\\azuremetadata.json"
		cmd := fmt.Sprintf("copy metadata_test.json %s", metadataFile)
		//nolint:errcheck // initial test setup
		p.ExecuteRawCommand(cmd)
	}

	hostu, _ := url.Parse("tcp://" + hostAgentUrl)
	hostAgent, err := common.NewListener(hostu)
	if err != nil {
		fmt.Printf("Failed to create agent, err:%v.\n", err)
		return
	}

	hostAgent.AddHandler("/", handleGetCloud)
	err = hostAgent.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}

	exitCode := m.Run()

	if runtime.GOOS == "linux" {
		//nolint:errcheck // test cleanup
		p.ExecuteRawCommand("rm /tmp/azuremetadata.json")
	} else {
		metadataFile := filepath.FromSlash(os.Getenv("TEMP")) + "\\azuremetadata.json"
		cmd := fmt.Sprintf("del %s", metadataFile)
		//nolint:errcheck // initial test cleanup
		p.ExecuteRawCommand(cmd)
	}

	log.Close()
	hostAgent.Stop()
	os.Exit(exitCode)
}

func handleGetCloud(w http.ResponseWriter, req *http.Request) {
	w.Write([]byte(getCloudResponse))
}

func TestEmptyAIKey(t *testing.T) {
	var err error

	aiConfig := AIConfig{
		AppName:                      "testapp",
		AppVersion:                   "v1.0.26",
		BatchSize:                    4096,
		BatchInterval:                2,
		RefreshTimeout:               10,
		DebugMode:                    true,
		DisableMetadataRefreshThread: true,
	}
	_, err = NewAITelemetry(httpURL, "", aiConfig)
	if err == nil {
		t.Errorf("Error initializing AI telemetry:%v", err)
	}

	_, err = NewAITelemetryWithConnectionString("", aiConfig)
	if err == nil {
		t.Errorf("Error initializing AI telemetry with connection string:%v", err)
	}
}

func TestNewAITelemetry(t *testing.T) {
	var err error

	aiConfig := AIConfig{
		AppName:                      "testapp",
		AppVersion:                   "v1.0.26",
		BatchSize:                    4096,
		BatchInterval:                2,
		RefreshTimeout:               10,
		GetEnvRetryCount:             1,
		GetEnvRetryWaitTimeInSecs:    2,
		DebugMode:                    true,
		DisableMetadataRefreshThread: true,
	}
	th1, err := NewAITelemetry(httpURL, "00ca2a73-c8d6-4929-a0c2-cf84545ec225", aiConfig)
	if th1 == nil {
		t.Errorf("Error initializing AI telemetry: %v", err)
	}

	th2, err := NewAITelemetryWithConnectionString(connectionString, aiConfig)
	if th2 == nil {
		t.Errorf("Error initializing AI telemetry with connection string: %v", err)
	}
}

func TestTrackMetric(t *testing.T) {
	metric := Metric{
		Name:             "test",
		Value:            1.0,
		CustomDimensions: make(map[string]string),
	}

	metric.CustomDimensions["dim1"] = "col1"
	th.TrackMetric(metric)
}

func TestTrackLog(t *testing.T) {
	report := Report{
		Message:          "test",
		Context:          "10a",
		CustomDimensions: make(map[string]string),
	}

	report.CustomDimensions["dim1"] = "col1"
	th.TrackLog(report)
}

func TestTrackEvent(t *testing.T) {
	event := Event{
		EventName:  "testEvent",
		ResourceID: "SomeResourceId",
		Properties: make(map[string]string),
	}

	event.Properties["P1"] = "V1"
	event.Properties["P2"] = "V2"
	th.TrackEvent(event)
}

func TestFlush(t *testing.T) {
	th.Flush()
}

func TestClose(t *testing.T) {
	th.Close(10)
}

func TestClosewithoutSend(t *testing.T) {
	var err error

	aiConfig := AIConfig{
		AppName:                      "testapp",
		AppVersion:                   "v1.0.26",
		BatchSize:                    4096,
		BatchInterval:                2,
		DisableMetadataRefreshThread: true,
		RefreshTimeout:               10,
		GetEnvRetryCount:             1,
		GetEnvRetryWaitTimeInSecs:    2,
	}

	thtest, err := NewAITelemetry(httpURL, "00ca2a73-c8d6-4929-a0c2-cf84545ec225", aiConfig)
	if thtest == nil {
		t.Errorf("Error initializing AI telemetry:%v", err)
	}

	thtest2, err := NewAITelemetryWithConnectionString(connectionString, aiConfig)
	if thtest2 == nil {
		t.Errorf("Error initializing AI telemetry with connection string:%v", err)
	}

	thtest.Close(10)
	thtest2.Close(10)
}
