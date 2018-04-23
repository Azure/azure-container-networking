// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
)

// OS Details structure.
type OSInfo struct {
	OSType         string
	OSVersion      string
	KernelVersion  string
	OSDistribution string
	ErrorMessage   string
}

// System Details structure.
type SystemInfo struct {
	MemVMTotal       uint64
	MemVMFree        uint64
	MemUsedByProcess uint64
	DiskVMTotal      uint64
	DiskVMFree       uint64
	CPUCount         int
	ErrorMessage     string
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
	ErrorMessage          string
}

// CNI Bridge Details structure.
type BridgeInfo struct {
	NetworkMode  string
	BridgeName   string
	ErrorMessage string
}

// Orchestrator Details structure.
type OrchestratorInfo struct {
	OrchestratorName    string
	OrchestratorVersion string
	ErrorMessage        string
}

// Azure CNI Telemetry Report structure.
type Report struct {
	StartFlag           bool
	CniSucceeded        bool
	Name                string
	Version             string
	ErrorMessage        string
	Context             string
	SubContext          string
	VnetAddressSpace    []string
	OrchestratorDetails *OrchestratorInfo
	OSDetails           *OSInfo
	SystemDetails       *SystemInfo
	InterfaceDetails    *InterfaceInfo
	BridgeDetails       *BridgeInfo
}

// ReportManager structure.
type ReportManager struct {
	HostNetAgentURL string
	IpamQueryURL    string
	ReportType      string
	Report          *Report
}

const (
	// TelemetryFile Path.
	TelemetryFile = platform.CNIRuntimePath + "AzureCNITelemetry.json"
)

// Read file line by line and return array of lines.
func ReadFileByLines(filename string) ([]string, error) {
	var (
		lineStrArr []string
	)

	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Error opening %s file error %v", filename, err)
	}

	r := bufio.NewReader(f)

	for {
		lineStr, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("Error reading %s file error %v", filename, err)
			}
			break
		}
		lineStrArr = append(lineStrArr, lineStr)
	}

	err = f.Close()
	if err != nil {
		return nil, fmt.Errorf("Error closing %s file error %v", filename, err)
	}

	return lineStrArr, nil
}

// GetReport retrieves orchestrator, system, OS and Interface details and create a report structure.
func (reportMgr *ReportManager) GetReport(name string, version string) {
	reportMgr.Report.Name = name
	reportMgr.Report.Version = version

	reportMgr.Report.GetOrchestratorDetails()
	reportMgr.Report.GetSystemDetails()
	reportMgr.Report.GetOSDetails()
	reportMgr.Report.GetInterfaceDetails(reportMgr.IpamQueryURL)
}

// This function will send telemetry report to HostNetAgent.
func (reportMgr *ReportManager) SendReport() error {
	var body bytes.Buffer

	httpc := &http.Client{}
	json.NewEncoder(&body).Encode(reportMgr.Report)

	log.Printf("Going to send Telemetry report to hostnetagent %v", reportMgr.HostNetAgentURL)

	log.Printf(`"Start Flag %t CniSucceeded %t Name %v Version %v ErrorMessage %v vnet %v 
				Context %v SubContext %v"`, reportMgr.Report.StartFlag, reportMgr.Report.CniSucceeded, reportMgr.Report.Name,
		reportMgr.Report.Version, reportMgr.Report.ErrorMessage, reportMgr.Report.VnetAddressSpace,
		reportMgr.Report.Context, reportMgr.Report.SubContext)

	log.Printf("OrchestratorDetails %v", reportMgr.Report.OrchestratorDetails)
	log.Printf("OSDetails %v", reportMgr.Report.OSDetails)
	log.Printf("SystemDetails %v", reportMgr.Report.SystemDetails)
	log.Printf("InterfaceDetails %v", reportMgr.Report.InterfaceDetails)
	log.Printf("BridgeDetails %v", reportMgr.Report.BridgeDetails)

	res, err := httpc.Post(reportMgr.HostNetAgentURL, reportMgr.ReportType, &body)
	if err != nil {
		return fmt.Errorf("[Azure CNI] HTTP Post returned error %v", err)
	}

	if res.StatusCode != 200 {
		if res.StatusCode == 400 {
			return fmt.Errorf(`"[Azure CNI] HTTP Post returned statuscode %d. 
				This error happens because telemetry service is not yet activated. 
				The error can be ignored as it won't affect CNI functionality"`, res.StatusCode)
		}

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

// This function  creates a report with interface details(ip, mac, name, secondaryca count).
func (report *Report) GetInterfaceDetails(queryUrl string) {
	var (
		macAddress       string
		secondaryCACount int
		primaryCA        string
		subnet           string
		ifName           string
	)

	if queryUrl == "" {
		report.InterfaceDetails.ErrorMessage = "IpamQueryUrl is null"
		return
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		report.InterfaceDetails = &InterfaceInfo{}
		report.InterfaceDetails.ErrorMessage = "Getting all interfaces failed due to " + err.Error()
		return
	}

	resp, err := http.Get(queryUrl)
	if err != nil {
		report.InterfaceDetails = &InterfaceInfo{}
		report.InterfaceDetails.ErrorMessage = "Http get failed in getting interface details " + err.Error()
		return
	}

	defer resp.Body.Close()

	// Decode XML document.
	var doc common.XmlDocument
	decoder := xml.NewDecoder(resp.Body)
	err = decoder.Decode(&doc)
	if err != nil {
		report.InterfaceDetails = &InterfaceInfo{}
		report.InterfaceDetails.ErrorMessage = "xml decode failed due to " + err.Error()
		return
	}

	// For each interface...
	for _, i := range doc.Interface {
		i.MacAddress = strings.ToLower(i.MacAddress)

		if i.IsPrimary {
			// Find the interface with the matching MacAddress.
			for _, iface := range interfaces {
				macAddr := strings.Replace(iface.HardwareAddr.String(), ":", "", -1)
				macAddr = strings.ToLower(macAddr)
				if macAddr == i.MacAddress {
					ifName = iface.Name
					macAddress = iface.HardwareAddr.String()
				}
			}

			for _, s := range i.IPSubnet {
				for _, ip := range s.IPAddress {
					if ip.IsPrimary {
						primaryCA = ip.Address
						subnet = s.Prefix
					} else {
						secondaryCACount += 1
					}
				}
			}

			break
		}
	}

	report.InterfaceDetails = &InterfaceInfo{
		InterfaceType:         "Primary",
		MAC:                   macAddress,
		Subnet:                subnet,
		Name:                  ifName,
		PrimaryCA:             primaryCA,
		SecondaryCATotalCount: secondaryCACount,
	}
}

// This function  creates a report with orchestrator details(name, version).
func (report *Report) GetOrchestratorDetails() {
	out, err := exec.Command("kubectl", "--version").Output()
	if err != nil {
		report.OrchestratorDetails = &OrchestratorInfo{}
		report.OrchestratorDetails.ErrorMessage = "kubectl command failed due to " + err.Error()
		return
	}

	outStr := string(out)
	outStr = strings.TrimLeft(outStr, " ")
	report.OrchestratorDetails = &OrchestratorInfo{}

	resultArray := strings.Split(outStr, " ")
	if len(resultArray) >= 2 {
		report.OrchestratorDetails.OrchestratorName = resultArray[0]
		report.OrchestratorDetails.OrchestratorVersion = resultArray[1]
	} else {
		report.OrchestratorDetails.ErrorMessage = "Length of array is less than 2"
	}
}
