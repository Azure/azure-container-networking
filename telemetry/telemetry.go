// Copyright 2018 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/azure-container-networking/aitelemetry"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	// CNITelemetryFile Path.
	CNITelemetryFile = platform.CNIRuntimePath + "AzureCNITelemetry.json"
	// ContentType of JSON
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

// Azure CNI Telemetry Report structure.
type CNIReport struct {
	IsNewInstance     bool
	CniSucceeded      bool
	Name              string
	Version           string
	ErrorMessage      string
	EventMessage      string
	OperationType     string
	OperationDuration int
	Context           string
	SubContext        string
	VMUptime          string
	Timestamp         string
	ContainerName     string
	InfraVnetID       string
	VnetAddressSpace  []string
	OSDetails         OSInfo
	SystemDetails     SystemInfo
	InterfaceDetails  InterfaceInfo
	BridgeDetails     BridgeInfo
	Metadata          common.Metadata `json:"compute"`
}

type AIMetric struct {
	Metric aitelemetry.Metric
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
