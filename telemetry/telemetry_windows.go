// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

type MemInfo struct {
	MemTotal uint64
	MemFree  uint64
}

type DiskInfo struct {
	DiskTotal uint64
	DiskFree  uint64
}

const (
	TelemetryFile = "c:\\AzureCNITelemetry"
)

func getMemInfo() (*MemInfo, error) {

	return nil, nil
}

func getDiskInfo(path string) (*DiskInfo, error) {

	return nil, nil
}

func (report *Report) GetSystemDetails() {

}

func (report *Report) GetOSDetails() {

}
