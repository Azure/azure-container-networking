// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	telemetryConfig = "azure-vnet-telemetry.config"
)

var reportManager *ReportManager
var tb *TelemetryBuffer
var ipamQueryUrl = "localhost:3501"
var hostAgentUrl = "localhost:3500"

var ipamQueryResponse = "" +
	"<Interfaces>" +
	"	<Interface MacAddress=\"*\" IsPrimary=\"true\">" +
	"		<IPSubnet Prefix=\"10.0.0.0/16\">" +
	"			<IPAddress Address=\"10.0.0.4\" IsPrimary=\"true\"/>" +
	"			<IPAddress Address=\"10.0.0.5\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.6\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.7\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.8\" IsPrimary=\"false\"/>" +
	"			<IPAddress Address=\"10.0.0.9\" IsPrimary=\"false\"/>" +
	"		</IPSubnet>" +
	"	</Interface>" +
	"</Interfaces>"

var sampleCniReport = CNIReport{
	IsNewInstance: false,
	EventMessage:  "[azure-cns] Code:UnknownContainerID {IPConfiguration:{IPSubnet:{IPAddress: PrefixLength:0} DNSServers:[] GatewayIPAddress:} Routes:[] CnetAddressSpace:[] MultiTenancyInfo:{EncapType: ID:0} PrimaryInterfaceIdentifier: LocalIPConfiguration:{IPSubnet:{IPAddress: PrefixLength:0} DNSServers:[] GatewayIPAddress:} {ReturnCode:18 Message:NetworkContainer doesn't exist.}}.",
	Timestamp:     "2019-02-27 17:44:47.319911225 +0000 UTC",
	Metadata: common.Metadata{
		Location:             "EastUS2EUAP",
		VMName:               "k8s-agentpool1-65609007-0",
		Offer:                "aks",
		OsType:               "Linux",
		PlacementGroupID:     "",
		PlatformFaultDomain:  "0",
		PlatformUpdateDomain: "0",
		Publisher:            "microsoft-aks",
		ResourceGroupName:    "rghostnetagttest",
		Sku:                  "aks-ubuntu-1604-201811",
		SubscriptionID:       "eff73b63-f38d-4cb5-bad1-21f273c1e36b",
		Tags:                 "acsengineVersion:v0.25.0;creationSource:acsengine-k8s-agentpool1-65609007-0;orchestrator:Kubernetes:1.10.9;poolName:agentpool1;resourceNameSuffix:65609007",
		OSVersion:            "2018.11.02",
		VMID:                 "eff73b63-f38d-4cb5-bad1-21f273c1e36b",
		VMSize:               "Standard_DS2_v2",
		KernelVersion:        ""}}

func TestMain(m *testing.M) {
	u, _ := url.Parse("tcp://" + ipamQueryUrl)
	ipamAgent, err := common.NewListener(u)
	if err != nil {
		fmt.Printf("Failed to create agent, err:%v.\n", err)
		return
	}

	ipamAgent.AddHandler("/", handleIpamQuery)

	err = ipamAgent.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}

	hostu, _ := url.Parse("tcp://" + hostAgentUrl)
	hostAgent, err := common.NewListener(hostu)
	if err != nil {
		fmt.Printf("Failed to create agent, err:%v.\n", err)
		return
	}

	hostAgent.AddHandler("/", handlePayload)

	err = hostAgent.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}

	if runtime.GOOS == "linux" {
		platform.ExecuteCommand("cp metadata_test.json /tmp/azuremetadata.json")
	} else {
		platform.ExecuteCommand("copy metadata_test.json azuremetadata.json")
	}

	reportManager = &ReportManager{}
	reportManager.HostNetAgentURL = "http://" + hostAgentUrl
	reportManager.ContentType = "application/json"
	reportManager.Report = &CNIReport{}

	tb = NewTelemetryBuffer()
	err = tb.StartServer()
	if err == nil {
		go tb.PushData()
	}

	if err := tb.Connect(); err != nil {
		fmt.Printf("connection to telemetry server failed %v", err)
	}

	exitCode := m.Run()

	if runtime.GOOS == "linux" {
		platform.ExecuteCommand("rm /tmp/azuremetadata.json")
	} else {
		platform.ExecuteCommand("del azuremetadata.json")
	}

	tb.Cleanup(FdName)
	ipamAgent.Stop()
	os.Exit(exitCode)
}

// Handles queries from IPAM source.
func handleIpamQuery(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(ipamQueryResponse))
}

func handlePayload(rw http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	var t Buffer
	err := decoder.Decode(&t)
	if err != nil {
		panic(err)
	}
	defer req.Body.Close()
	log.Println(t)

	fmt.Printf("Buffer: %+v", t)
}

func TestGetOSDetails(t *testing.T) {
	reportManager.Report.(*CNIReport).GetOSDetails()
	if reportManager.Report.(*CNIReport).ErrorMessage != "" {
		t.Errorf("GetOSDetails failed due to %v", reportManager.Report.(*CNIReport).ErrorMessage)
	}
}
func TestGetSystemDetails(t *testing.T) {
	reportManager.Report.(*CNIReport).GetSystemDetails()
	if reportManager.Report.(*CNIReport).ErrorMessage != "" {
		t.Errorf("GetSystemDetails failed due to %v", reportManager.Report.(*CNIReport).ErrorMessage)
	}
}
func TestGetInterfaceDetails(t *testing.T) {
	reportManager.Report.(*CNIReport).GetSystemDetails()
	if reportManager.Report.(*CNIReport).ErrorMessage != "" {
		t.Errorf("GetInterfaceDetails failed due to %v", reportManager.Report.(*CNIReport).ErrorMessage)
	}
}

func TestGetReportState(t *testing.T) {
	state := reportManager.GetReportState(CNITelemetryFile)
	if state != false {
		t.Errorf("Wrong state in getreport state")
	}
}

func TestSendTelemetry(t *testing.T) {
	err := reportManager.SendReport(tb)
	if err != nil {
		t.Errorf("SendTelemetry failed due to %v", err)
	}

	i := 3
	rpMgr := &ReportManager{}
	rpMgr.Report = &i
	err = rpMgr.SendReport(tb)
	if err == nil {
		t.Errorf("SendTelemetry not failed for incorrect report type")
	}
}

func TestCloseTelemetryConnection(t *testing.T) {
	tb.Cancel()
	time.Sleep(300 * time.Millisecond)

	tb.mutex.Lock()
	if len(tb.connections) != 0 {
		t.Errorf("server didn't close connection")
	}
	tb.mutex.Unlock()
}

func TestServerCloseTelemetryConnection(t *testing.T) {
	// create server telemetrybuffer and start server
	tb = NewTelemetryBuffer()
	err := tb.StartServer()
	if err == nil {
		go tb.PushData()
	}

	// create client telemetrybuffer and connect to server
	tb1 := NewTelemetryBuffer()
	if err := tb1.Connect(); err != nil {
		t.Errorf("connection to telemetry server failed %v", err)
	}

	// Exit server thread and close server connection
	tb.Cancel()
	time.Sleep(300 * time.Millisecond)

	b := []byte("tamil")
	if _, err := tb1.Write(b); err == nil {
		t.Errorf("Client couldn't recognise server close")
	}

	tb.mutex.Lock()
	if len(tb.connections) != 0 {
		t.Errorf("All connections not closed as expected")
	}
	tb.mutex.Unlock()

	// Close client connection
	tb1.Close()
}

func TestClientCloseTelemetryConnection(t *testing.T) {
	// create server telemetrybuffer and start server
	tb = NewTelemetryBuffer()
	err := tb.StartServer()
	if err == nil {
		go tb.PushData()
	}

	if !SockExists() {
		t.Errorf("telemetry sock doesn't exist")
	}

	// create client telemetrybuffer and connect to server
	tb1 := NewTelemetryBuffer()
	if err := tb1.Connect(); err != nil {
		t.Errorf("connection to telemetry server failed %v", err)
	}

	// Close client connection
	tb1.Close()
	time.Sleep(300 * time.Millisecond)

	tb.mutex.Lock()
	if len(tb.connections) != 0 {
		t.Errorf("All connections not closed as expected")
	}
	tb.mutex.Unlock()

	// Exit server thread and close server connection
	tb.Cancel()
}

func TestReadConfigFile(t *testing.T) {
	config, err := ReadConfigFile(telemetryConfig)
	if err != nil {
		t.Errorf("Read telemetry config failed with error %v", err)
	}

	if config.ReportToHostIntervalInSeconds != 30 {
		t.Errorf("ReportToHostIntervalInSeconds not expected value. Got %d", config.ReportToHostIntervalInSeconds)
	}

	config, err = ReadConfigFile("a.config")
	if err == nil {
		t.Errorf("[Telemetry] Didn't throw not found error: %v", err)
	}

	config, err = ReadConfigFile("telemetry.go")
	if err == nil {
		t.Errorf("[Telemetry] Didn't report invalid telemetry config: %v", err)
	}
}

func TestStartTelemetryService(t *testing.T) {
	err := StartTelemetryService("", nil)
	if err == nil {
		t.Errorf("StartTelemetryService didnt return error for incorrect service name %v", err)
	}
}

func TestWaitForTelemetrySocket(t *testing.T) {
	WaitForTelemetrySocket(1, 10)
}

func TestSetReportState(t *testing.T) {
	err := reportManager.SetReportState("a.json")
	if err != nil {
		t.Errorf("SetReportState failed due to %v", err)
	}

	err = os.Remove("a.json")
	if err != nil {
		t.Errorf("Error removing telemetry file due to %v", err)
	}
}
