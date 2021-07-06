// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"fmt"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"

	"github.com/Azure/azure-container-networking/platform"
)

// Memory Info structure.
type MemInfo struct {
	MemTotal uint64
	MemFree  uint64
}

// Disk Info structure.
type DiskInfo struct {
	DiskTotal uint64
	DiskFree  uint64
}

const (
	MB = 1048576
	KB = 1024
)

// This function retrieves VMs memory usage.
func getMemInfo() (*MemInfo, error) {
	info := &syscall.Sysinfo_t{}

	err := syscall.Sysinfo(info)
	if err != nil {
		return nil, fmt.Errorf("Sysinfo failed due to %v", err)
	}

	unit := uint64(info.Unit) * MB //MB
	memInfo := &MemInfo{MemTotal: info.Totalram / unit, MemFree: info.Freeram / unit}

	return memInfo, nil
}

// This function retrieves VMs disk usage.
func getDiskInfo(path string) (*DiskInfo, error) {
	fs := syscall.Statfs_t{}

	err := syscall.Statfs(path, &fs)
	if err != nil {
		return nil, fmt.Errorf("Statfs call failed with error %v", err)
	}

	total := fs.Blocks * uint64(fs.Bsize) / MB
	free := fs.Bfree * uint64(fs.Bsize) / MB
	diskInfo := &DiskInfo{DiskTotal: total, DiskFree: free}

	return diskInfo, nil
}

// This function  creates a report with system details(memory, disk, cpu).
func (report *CNIReport) GetSystemDetails() {
	var errMsg string

	cpuCount := runtime.NumCPU()

	memInfo, err := getMemInfo()
	if err != nil {
		errMsg = err.Error()
	}

	diskInfo, err := getDiskInfo("/")
	if err != nil {
		errMsg = errMsg + err.Error()
	}

	report.SystemDetails = SystemInfo{
		MemVMTotal:   memInfo.MemTotal,
		MemVMFree:    memInfo.MemFree,
		DiskVMTotal:  diskInfo.DiskTotal,
		DiskVMFree:   diskInfo.DiskFree,
		CPUCount:     cpuCount,
		ErrorMessage: errMsg,
	}
}

// This function  creates a report with os details(ostype, version).
func (report *CNIReport) GetOSDetails() {
	osInfoArr, err := platform.GetOSDetails()
	if err != nil {
		report.OSDetails = OSInfo{OSType: runtime.GOOS}
		report.OSDetails.ErrorMessage = "GetOSDetails failed with" + err.Error()
		return
	}

	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		report.OSDetails = OSInfo{OSType: runtime.GOOS}
		report.OSDetails.ErrorMessage = "uname -r failed with " + err.Error()
		return
	}

	kernelVersion := string(out)
	kernelVersion = strings.TrimSuffix(kernelVersion, "\n")

	report.OSDetails = OSInfo{
		OSType:         runtime.GOOS,
		OSVersion:      osInfoArr["VERSION"],
		KernelVersion:  kernelVersion,
		OSDistribution: osInfoArr["ID"],
	}
}

// Get kernel version
func (reportMgr *ReportManager) GetKernelVersion() {
	out, err := exec.Command("uname", "-r").Output()
	if err == nil {
		v := reflect.ValueOf(reportMgr.Report).Elem().FieldByName("Metadata")
		if v.CanSet() {
			v.FieldByName("KernelVersion").SetString(strings.TrimSuffix(string(out), "\n"))
		}
	}
}
