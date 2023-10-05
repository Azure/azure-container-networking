// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"runtime"
	"strings"

	"github.com/Azure/azure-container-networking/platform"
)

const (
	delimiter  = "\r\n"
	versionCmd = "ver"
)

type MemInfo struct {
	MemTotal uint64
	MemFree  uint64
}

type DiskInfo struct {
	DiskTotal uint64
	DiskFree  uint64
}

func getMemInfo() (*MemInfo, error) {
	return nil, nil
}

func getDiskInfo(path string) (*DiskInfo, error) {
	return nil, nil
}

func (report *CNIReport) GetSystemDetails() {
	report.SystemDetails = SystemInfo{}
}

func (report *CNIReport) GetOSDetails() {
	p := platform.NewExecClient(report.Logger)
	report.OSDetails = OSInfo{OSType: runtime.GOOS}
	out, err := p.ExecuteCommand(versionCmd)
	if err == nil {
		report.OSDetails.OSVersion = strings.Replace(out, delimiter, "", -1)
	}
}
