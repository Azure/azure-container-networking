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
	"testing"

	"github.com/Azure/azure-container-networking/common"
)

var reportManager *ReportManager
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

	hostAgent.AddHandler("/", handleCNIReport)

	err = hostAgent.Start(make(chan error, 1))
	if err != nil {
		fmt.Printf("Failed to start agent, err:%v.\n", err)
		return
	}

	reportManager = &ReportManager{}
	reportManager.HostNetAgentURL = "http://" + hostAgentUrl
	reportManager.ContentType = "application/json"
	reportManager.Report = &CNIReport{}
	exitCode := m.Run()
	os.Exit(exitCode)
}

// Handles queries from IPAM source.
func handleIpamQuery(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(ipamQueryResponse))
}

func handleCNIReport(rw http.ResponseWriter, req *http.Request) {
	decoder := json.NewDecoder(req.Body)
	var t CNIReport
	err := decoder.Decode(&t)
	if err != nil {
		panic(err)
	}
	defer req.Body.Close()
	log.Println(t)

	log.Println("OrchestratorDetails", t.OrchestratorDetails)
	log.Println("OSDetails", t.OSDetails)
	log.Println("SystemDetails", t.SystemDetails)
	log.Println("InterfaceDetails", t.InterfaceDetails)
	log.Println("BridgeDetails", t.BridgeDetails)
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
	err := reportManager.SendReport()
	if err != nil {
		t.Errorf("SendTelemetry failed due to %v", err)
	}
}

func TestSetReportState(t *testing.T) {
	err := reportManager.SetReportState(CNITelemetryFile)
	if err != nil {
		t.Errorf("SetReportState failed due to %v", err)
	}

	err = os.Remove(CNITelemetryFile)
	if err != nil {
		t.Errorf("Error removing telemetry file due to %v", err)
	}
}
