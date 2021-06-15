// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// NPMTelemetryFile Path.
	NPMTelemetryFile = platform.NPMRuntimePath + "AzureNPMTelemetry.json"
	// CNITelemetryFile Path.
	CNITelemetryFile = platform.CNIRuntimePath + "AzureCNITelemetry.json"
	// ContentType of JSON
	ContentType = "application/json"
	metadataURL = "http://169.254.169.254/metadata/instance?api-version=2017-08-01&format=json"
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
type CNIReport struct {
	IsNewInstance       bool
	CniSucceeded        bool
	Name                string
	Version             string
	ErrorMessage        string
	EventMessage        string
	OperationType       string
	OperationDuration   int
	Context             string
	SubContext          string
	VMUptime            string
	Timestamp           string
	ContainerName       string
	InfraVnetID         string
	VnetAddressSpace    []string
	OrchestratorDetails OrchestratorInfo
	OSDetails           OSInfo
	SystemDetails       SystemInfo
	InterfaceDetails    InterfaceInfo
	BridgeDetails       BridgeInfo
	Metadata            common.Metadata `json:"compute"`
}

type AIMetric struct {
	Metric aitelemetry.Metric
}

// ClusterState contains the current kubernetes cluster state.
type ClusterState struct {
	PodCount      int
	NsCount       int
	NwPolicyCount int
}

// NPMReport structure.
type NPMReport struct {
	IsNewInstance     bool
	ClusterID         string
	NodeName          string
	InstanceName      string
	NpmVersion        string
	KubernetesVersion string
	ErrorMessage      string
	EventMessage      string
	UpTime            string
	Timestamp         string
	ClusterState      ClusterState
	Metadata          common.Metadata `json:"compute"`
}

// ReportManager structure.
type ReportManager struct {
	HostNetAgentURL string
	ContentType     string
	Report          interface{}
}

// GetReport retrieves orchestrator, system, OS and Interface details and create a report structure.
func (report *CNIReport) GetReport(name string, version string, ipamQueryURL string) {
	report.Name = name
	report.Version = version

	report.GetSystemDetails()
	report.GetOSDetails()
}

// GetReport retrives npm and kubernetes cluster related info and create a report structure.
func (report *NPMReport) GetReport(clusterID, nodeName, npmVersion, kubernetesVersion string, clusterState ClusterState) {
	report.ClusterID = clusterID
	report.NodeName = nodeName
	report.NpmVersion = npmVersion
	report.KubernetesVersion = kubernetesVersion
	report.ClusterState = clusterState
}

// SendReport will send telemetry report to HostNetAgent.
func (reportMgr *ReportManager) SendReport(tb *TelemetryBuffer) error {
	var err error
	var report []byte

	if tb != nil && tb.Connected {
		report, err = reportMgr.ReportToBytes()
		if err == nil {
			// If write fails, try to re-establish connections as server/client
			if _, err = tb.Write(report); err != nil {
				tb.Cancel()
			}
		}
	}

	return err
}

// SetReportState will save the state in file if telemetry report sent successfully.
func (reportMgr *ReportManager) SetReportState(telemetryFile string) error {
	var reportBytes []byte
	var err error

	reportBytes, err = json.Marshal(reportMgr.Report)
	if err != nil {
		return fmt.Errorf("[Telemetry] report write failed with err %+v", err)
	}

	// try to open telemetry file
	f, err := os.OpenFile(telemetryFile, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("[Telemetry] Error opening telemetry file %v", err)
	}

	defer f.Close()

	_, err = f.Write(reportBytes)
	if err != nil {
		fmt.Printf("[Telemetry] Error while writing to file %v", err)
		return fmt.Errorf("[Telemetry] Error while writing to file %v", err)
	}

	// set IsNewInstance in report
	reflect.ValueOf(reportMgr.Report).Elem().FieldByName("IsNewInstance").SetBool(false)
	return nil
}

// GetReportState will check if report is sent at least once by checking telemetry file.
func (reportMgr *ReportManager) GetReportState(telemetryFile string) bool {
	// try to set IsNewInstance in report
	if _, err := os.Stat(telemetryFile); os.IsNotExist(err) {
		fmt.Printf("[Telemetry] File not exist %v", telemetryFile)
		reflect.ValueOf(reportMgr.Report).Elem().FieldByName("IsNewInstance").SetBool(true)
		return false
	}

	return true
}

// GetInterfaceDetails creates a report with interface details(ip, mac, name, secondaryca count).
func (report *CNIReport) GetInterfaceDetails(queryUrl string) {
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
		report.InterfaceDetails.ErrorMessage = "Getting all interfaces failed due to " + err.Error()
		return
	}

	resp, err := http.Get(queryUrl)
	if err != nil {
		report.InterfaceDetails.ErrorMessage = "Http get failed in getting interface details " + err.Error()
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errMsg := fmt.Sprintf("Error while getting interface details. http code :%d", resp.StatusCode)
		report.InterfaceDetails.ErrorMessage = errMsg
		log.Logf(errMsg)
		return
	}

	// Decode XML document.
	var doc common.XmlDocument
	decoder := xml.NewDecoder(resp.Body)
	err = decoder.Decode(&doc)
	if err != nil {
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
						secondaryCACount++
					}
				}
			}

			break
		}
	}

	report.InterfaceDetails = InterfaceInfo{
		InterfaceType:         "Primary",
		MAC:                   macAddress,
		Subnet:                subnet,
		Name:                  ifName,
		PrimaryCA:             primaryCA,
		SecondaryCATotalCount: secondaryCACount,
	}
}

// GetOrchestratorDetails creates a report with orchestrator details(name, version).
func (report *CNIReport) GetOrchestratorDetails() {
	// to-do: GetOrchestratorDetails for all report types and for all k8s environments
	// current implementation works for clusters created via acs-engine and on master nodes
	report.OrchestratorDetails = OrchestratorInfo{}

	// Check for orchestrator tag first
	for _, tag := range strings.Split(report.Metadata.Tags, ";") {
		if strings.Contains(tag, "orchestrator") {
			details := strings.Split(tag, ":")
			if len(details) != 2 {
				report.OrchestratorDetails.ErrorMessage = "length of orchestrator tag is less than 2"
			} else {
				report.OrchestratorDetails.OrchestratorName = details[0]
				report.OrchestratorDetails.OrchestratorVersion = details[1]
			}
		} else {
			report.OrchestratorDetails.ErrorMessage = "Host metadata unavailable"
		}
	}

	if report.OrchestratorDetails.ErrorMessage != "" {
		out, err := exec.Command("kubectl", "version").Output()
		if err != nil {
			report.OrchestratorDetails.ErrorMessage = "kubectl command failed due to " + err.Error()
			return
		}

		resultArray := strings.Split(strings.TrimLeft(string(out), " "), " ")
		if len(resultArray) >= 2 {
			report.OrchestratorDetails.OrchestratorName = resultArray[0]
			report.OrchestratorDetails.OrchestratorVersion = resultArray[1]
		} else {
			report.OrchestratorDetails.ErrorMessage = "Length of array is less than 2"
		}
	}
}

// ReportToBytes - returns the report bytes
func (reportMgr *ReportManager) ReportToBytes() ([]byte, error) {
	var err error
	var report []byte

	switch reportMgr.Report.(type) {
	case *CNIReport:
	case *AIMetric:
	default:
		err = fmt.Errorf("[Telemetry] Invalid report type")
	}

	if err != nil {
		return []byte{}, err
	}

	report, err = json.Marshal(reportMgr.Report)
	return report, err
}

// This function for sending CNI metrics to telemetry service
func SendCNIMetric(cniMetric *AIMetric, tb *TelemetryBuffer) error {
	var (
		err    error
		report []byte
	)

	if tb != nil && tb.Connected {
		reportMgr := &ReportManager{Report: cniMetric}
		report, err = reportMgr.ReportToBytes()
		if err == nil {
			// If write fails, try to re-establish connections as server/client
			if _, err = tb.Write(report); err != nil {
				tb.Cancel()
			}
		}
	}

	return err
}
