// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
)

type xmlDocument struct {
	XMLName   xml.Name `xml:"Interfaces"`
	Interface []struct {
		XMLName    xml.Name `xml:"Interface"`
		MacAddress string   `xml:"MacAddress,attr"`
		IsPrimary  bool     `xml:"IsPrimary,attr"`

		IPSubnet []struct {
			XMLName xml.Name `xml:"IPSubnet"`
			Prefix  string   `xml:"Prefix,attr"`

			IPAddress []struct {
				XMLName   xml.Name `xml:"IPAddress"`
				Address   string   `xml:"Address,attr"`
				IsPrimary bool     `xml:"IsPrimary,attr"`
			}
		}
	}
}

// OS Details structure.
type OSInfo struct {
	OSType         string
	OSVersion      string
	KernelVersion  string
	OSDistribution string
}

// System Details structure.
type SystemInfo struct {
	MemVMTotal       uint64
	MemVMFree        uint64
	MemUsedByProcess uint64
	DiskVMTotal      uint64
	DiskVMFree       uint64
	CPUCount         int
}

// Interface Details structure.
type InterfaceInfo struct {
	InterfaceType         string
	Subnet                string
	PrimaryCA             string
	MAC                   string
	Name                  string
	SecondaryCATotalCount int
	SecondaryCAUsedCount  int
}

// CNI Bridge Details structure.
type BridgeInfo struct {
	NetworkMode string
	BridgeName  string
}

type OrchsestratorInfo struct {
	OrchestratorName    string
	OrchestratorVersion string
}

// Azure CNI Telemetry Report structure.
type Report struct {
	StartFlag           bool
	Name                string
	Version             string
	OrchestratorDetails *OrchsestratorInfo
	OSDetails           *OSInfo
	SystemDetails       *SystemInfo
	InterfaceDetails    *InterfaceInfo
	BridgeDetails       *BridgeInfo
	VnetAddressSpace    []string
	ErrorMessage        string
	Context             string
	SubContext          string
}

// ReportManager structure.
type ReportManager struct {
	HostNetAgentURL string
	IpamQueryURL    string
	ReportType      string
	Report          *Report
}

// GetReport retrieves orchestrator, system, OS and Interface details and create a report structure.
func (reportMgr *ReportManager) GetReport(name string, version string) (*Report, error) {
	var err error
	report := &Report{}

	report.Name = name
	report.Version = version
	report.OrchestratorDetails = GetOrchestratorDetails()

	report.SystemDetails, err = GetSystemDetails()
	if err != nil {
		return report, err
	}

	report.OSDetails, err = GetOSDetails()
	if err != nil {
		return report, err
	}

	report.InterfaceDetails, err = GetInterfaceDetails(reportMgr.IpamQueryURL)
	if err != nil {
		return report, err
	}

	return report, nil
}

// This function will send telemetry report to HostNetAgent.
func (reportMgr *ReportManager) SendReport() error {
	var body bytes.Buffer

	httpc := &http.Client{}
	json.NewEncoder(&body).Encode(reportMgr.Report)

	log.Printf("Going to send Telemetry report to hostnetagent %v", reportMgr.HostNetAgentURL)
	log.Printf("Flag %v Name %v Version %v orchestname %s ErrorMessage %v SystemDetails %v OSDetails %v InterfaceDetails %v",
		reportMgr.Report.StartFlag, reportMgr.Report.Name, reportMgr.Report.Version, reportMgr.Report.OrchestratorDetails, reportMgr.Report.ErrorMessage,
		reportMgr.Report.SystemDetails, reportMgr.Report.OSDetails, reportMgr.Report.InterfaceDetails)

	res, err := httpc.Post(reportMgr.HostNetAgentURL, reportMgr.ReportType, &body)
	if err != nil {
		return fmt.Errorf("[Azure CNI] HTTP Post returned error %v", err)
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("[Azure CNI] HTTP Post returned statuscode %d", res.StatusCode)
	}

	log.Printf("Send telemetry success %d\n", res.StatusCode)
	return nil
}

// This function will save the state in file if telemetry report sent successfully.
func (report *Report) SetReportState() error {
	f, err := os.OpenFile(TelemetryFile, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("Error opening telemetry file %v", err)
	}

	defer f.Close()

	reportBytes, err := json.Marshal(report)
	if err != nil {
		log.Printf("report write failed due to %v", err)
		_, err = f.WriteString("report write failed")
	} else {
		_, err = f.Write(reportBytes)
	}

	if err != nil {
		log.Printf("Error while writing to file %v", err)
		return fmt.Errorf("Error while writing to file %v", err)
	}

	report.StartFlag = false
	log.Printf("SetReportState succeeded")
	return nil
}

// This function will check if report is sent atleast once by checking telemetry file.
func (report *Report) GetReportState() bool {
	if _, err := os.Stat(TelemetryFile); os.IsNotExist(err) {
		log.Printf("File not exist %v", TelemetryFile)
		report.StartFlag = true
		return false
	}

	return true
}
