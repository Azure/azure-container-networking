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
	aiConfig         AIConfig
	hostAgentUrl     = "localhost:3501"
	getCloudResponse = "AzurePublicCloud"
	httpURL          = "http://" + hostAgentUrl
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

	aiConfig = AIConfig{
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

func initTelemetry(_ *testing.T) (th1, th2 TelemetryHandle) {
	th1, err1 := NewAITelemetry(httpURL, "00ca2a73-c8d6-4929-a0c2-cf84545ec225", aiConfig)
	if err1 != nil {
		fmt.Printf("Error initializing AI telemetry: %v", err1)
	}

	th2, err2 := NewWithConnectionString(connectionString, aiConfig)
	if err2 != nil {
		fmt.Printf("Error initializing AI telemetry with connection string: %v", err2)
	}

	return
}

func TestEmptyAIKey(t *testing.T) {
	var err error

	_, err = NewAITelemetry(httpURL, "", aiConfig)
	if err == nil {
		t.Errorf("Error initializing AI telemetry:%v", err)
	}

	_, err = NewWithConnectionString("", aiConfig)
	if err == nil {
		t.Errorf("Error initializing AI telemetry with connection string:%v", err)
	}
}

func TestNewAITelemetry(t *testing.T) {
	var err error

	th1, th2 := initTelemetry(t)
	if th1 == nil {
		t.Errorf("Error initializing AI telemetry: %v", err)
	}

	if th2 == nil {
		t.Errorf("Error initializing AI telemetry with connection string: %v", err)
	}
}

func TestTrackMetric(t *testing.T) {
	th1, th2 := initTelemetry(t)

	metric := Metric{
		Name:             "test",
		Value:            1.0,
		CustomDimensions: make(map[string]string),
	}

	metric.CustomDimensions["dim1"] = "col1"
	th1.TrackMetric(metric)
	th2.TrackMetric(metric)
}

func TestTrackLog(t *testing.T) {
	th1, th2 := initTelemetry(t)

	report := Report{
		Message:          "test",
		Context:          "10a",
		CustomDimensions: make(map[string]string),
	}

	report.CustomDimensions["dim1"] = "col1"
	th1.TrackLog(report)
	th2.TrackLog(report)
}

func TestTrackEvent(t *testing.T) {
	th1, th2 := initTelemetry(t)

	event := Event{
		EventName:  "testEvent",
		ResourceID: "SomeResourceId",
		Properties: make(map[string]string),
	}

	event.Properties["P1"] = "V1"
	event.Properties["P2"] = "V2"
	th1.TrackEvent(event)
	th2.TrackEvent(event)
}

func TestFlush(t *testing.T) {
	th1, th2 := initTelemetry(t)

	th1.Flush()
	th2.Flush()
}

func TestClose(t *testing.T) {
	th1, th2 := initTelemetry(t)

	th1.Close(10)
	th2.Close(10)
}
