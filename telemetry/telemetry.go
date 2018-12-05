// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// NPMTelemetryFile Path.
	NPMTelemetryFile = platform.NPMRuntimePath + "AzureNPMTelemetry.json"
	// CNITelemetryFile Path.
	CNITelemetryFile = platform.CNIRuntimePath + "AzureCNITelemetry.json"

	metadataURL = "http://169.254.169.254/metadata/instance?api-version=2017-08-01&format=json"
	ContentType = "application/json"
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

// Metadata retrieved from wireserver
type Metadata struct {
	Location             string `json:"location"`
	VMName               string `json:"name"`
	Offer                string `json:"offer"`
	OsType               string `json:"osType"`
	PlacementGroupID     string `json:"placementGroupId"`
	PlatformFaultDomain  string `json:"platformFaultDomain"`
	PlatformUpdateDomain string `json:"platformUpdateDomain"`
	Publisher            string `json:"publisher"`
	ResourceGroupName    string `json:"resourceGroupName"`
	Sku                  string `json:"sku"`
	SubscriptionID       string `json:"subscriptionId"`
	Tags                 string `json:"tags"`
	OSVersion            string `json:"version"`
	VMID                 string `json:"vmId"`
	VMSize               string `json:"vmSize"`
	KernelVersion        string
}

type metadataWrapper struct {
	Metadata Metadata `json:"compute"`
}

// Azure CNI Telemetry Report structure.
type CNIReport struct {
	IsNewInstance       bool
	CniSucceeded        bool
	Name                string
	OSVersion           string
	ErrorMessage        string
	Context             string
	SubContext          string
	VnetAddressSpace    []string
	OrchestratorDetails *OrchestratorInfo
	OSDetails           *OSInfo
	SystemDetails       *SystemInfo
	InterfaceDetails    *InterfaceInfo
	BridgeDetails       *BridgeInfo
	Metadata            Metadata `json:"compute"`
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
	ClusterState      ClusterState
	Metadata          Metadata `json:"compute"`
}

// DNCReport structure.
type DNCReport struct {
	IsNewInstance bool
	CPUUsage      string
	MemoryUsage   string
	Processes     string
	EventMessage  string
	PartitionKey  string
	Allocations   string
	Timestamp     string
	UUID          string
	Errorcode     string
	Metadata      Metadata `json:"compute"`
}

// ReportManager structure.
type ReportManager struct {
	HostNetAgentURL string
	ContentType     string
	Report          interface{}
}

// ReadFileByLines reads file line by line and return array of lines.
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
func (report *CNIReport) GetReport(name string, version string, ipamQueryURL string) {
	report.Name = name
	report.OSVersion = version

	report.GetOrchestratorDetails()
	report.GetSystemDetails()
	report.GetOSDetails()
	report.GetInterfaceDetails(ipamQueryURL)
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
func (reportMgr *ReportManager) SendReport() error {
	log.Printf("[Telemetry] Going to send Telemetry report to hostnetagent %v", reportMgr.HostNetAgentURL)

	switch reportMgr.Report.(type) {
	case *CNIReport:
		log.Printf("[Telemetry] %+v", reportMgr.Report.(*CNIReport))
	case *NPMReport:
		log.Printf("[Telemetry] %+v", reportMgr.Report.(*NPMReport))
	case *DNCReport:
		log.Printf("[Telemetry] %+v", reportMgr.Report.(*DNCReport))
	default:
		log.Printf("[Telemetry] Invalid report type")
	}

	httpc := &http.Client{}
	var body bytes.Buffer
	json.NewEncoder(&body).Encode(reportMgr.Report)
	resp, err := httpc.Post(reportMgr.HostNetAgentURL, reportMgr.ContentType, &body)
	if err != nil {
		return fmt.Errorf("[Telemetry] HTTP Post returned error %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		if resp.StatusCode == 400 {
			return fmt.Errorf(`"[Telemetry] HTTP Post returned statuscode %d. 
				This error happens because telemetry service is not yet activated. 
				The error can be ignored as it won't affect functionality"`, resp.StatusCode)
		}

		return fmt.Errorf("[Telemetry] HTTP Post returned statuscode %d", resp.StatusCode)
	}

	log.Printf("[Telemetry] Telemetry sent with status code %d\n", resp.StatusCode)

	return nil
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
		log.Printf("[Telemetry] Error while writing to file %v", err)
		return fmt.Errorf("[Telemetry] Error while writing to file %v", err)
	}

	// set IsNewInstance in report
	reflect.ValueOf(reportMgr.Report).Elem().FieldByName("IsNewInstance").SetBool(false)
	log.Printf("[Telemetry] SetReportState succeeded")
	return nil
}

// GetReportState will check if report is sent at least once by checking telemetry file.
func (reportMgr *ReportManager) GetReportState(telemetryFile string) bool {
	// try to set IsNewInstance in report
	if _, err := os.Stat(telemetryFile); os.IsNotExist(err) {
		log.Printf("[Telemetry] File not exist %v", telemetryFile)
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
						secondaryCACount++
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

// GetOrchestratorDetails creates a report with orchestrator details(name, version).
func (report *CNIReport) GetOrchestratorDetails() {
	// to-do: GetOrchestratorDetails for all report types and for all k8s environments
	// current implementation works for clusters created via acs-engine and on master nodes
	report.OrchestratorDetails = &OrchestratorInfo{}

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

// GetHostMetadata - retrieve metadata from host
func (reportMgr *ReportManager) GetHostMetadata() error {
	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Metadata", "True")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("[Telemetry] Request failed with HTTP error %d", resp.StatusCode)
	} else if resp.Body != nil {
		report := metadataWrapper{}
		err = json.NewDecoder(resp.Body).Decode(&report)
		if err == nil {
			// Find Metadata struct in report and try to set values
			v := reflect.ValueOf(reportMgr.Report).Elem().FieldByName("Metadata")
			if v.CanSet() {
				v.FieldByName("Location").SetString(report.Metadata.Location)
				v.FieldByName("VMName").SetString(report.Metadata.VMName)
				v.FieldByName("Offer").SetString(report.Metadata.Offer)
				v.FieldByName("OsType").SetString(report.Metadata.OsType)
				v.FieldByName("PlacementGroupID").SetString(report.Metadata.PlacementGroupID)
				v.FieldByName("PlatformFaultDomain").SetString(report.Metadata.PlatformFaultDomain)
				v.FieldByName("PlatformUpdateDomain").SetString(report.Metadata.PlatformUpdateDomain)
				v.FieldByName("Publisher").SetString(report.Metadata.Publisher)
				v.FieldByName("ResourceGroupName").SetString(report.Metadata.ResourceGroupName)
				v.FieldByName("Sku").SetString(report.Metadata.Sku)
				v.FieldByName("SubscriptionID").SetString(report.Metadata.SubscriptionID)
				v.FieldByName("Tags").SetString(report.Metadata.Tags)
				v.FieldByName("OSVersion").SetString(report.Metadata.OSVersion)
				v.FieldByName("VMID").SetString(report.Metadata.VMID)
				v.FieldByName("VMSize").SetString(report.Metadata.VMSize)
			} else {
				err = fmt.Errorf("[Telemetry] Unable to set metadata values")
			}
		} else {
			err = fmt.Errorf("[Telemetry] Unable to decode response body due to error: %s", err.Error())
		}
	} else {
		err = fmt.Errorf("[Telemetry] Response body is empty")
	}

	return err
}

// ReportToBytes - returns the report bytes
func (reportMgr *ReportManager) ReportToBytes() (report []byte, err error) {
	switch reportMgr.Report.(type) {
	case *CNIReport:
	case *NPMReport:
	case *DNCReport:
	default:
		err = fmt.Errorf("[Telemetry] Invalid report type")
	}

	if err == nil {
		report, err = json.Marshal(reportMgr.Report)
	}

	return
}
