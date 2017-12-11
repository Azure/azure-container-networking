// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package telemetry

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/Azure/azure-container-networking/common"
)

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

func getMemInfo() (*MemInfo, error) {

	return nil, nil
}

func getDiskInfo(path string) (*DiskInfo, error) {

	return nil, nil
}

func GetSystemDetails() (*SystemInfo, error) {

	return nil, nil
}

func GetOSDetails() (*OSInfo, error) {

	return nil, nil
}

func GetInterfaceDetails(queryUrl string) (*InterfaceInfo, error) {

	var (
		macAddress       string
		secondaryCACount int
		primaryCA        string
		subnet           string
		ifName           string
	)

	if queryUrl == "" {
		return nil, fmt.Errorf("IpamQueryUrl is null")
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	resp, err := http.Get(queryUrl)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// Decode XML document.
	var doc common.XmlDocument
	decoder := xml.NewDecoder(resp.Body)
	err = decoder.Decode(&doc)
	if err != nil {
		return nil, err
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

	interfaceInfo := &InterfaceInfo{
		InterfaceType:         "Primary",
		MAC:                   macAddress,
		Subnet:                subnet,
		Name:                  ifName,
		PrimaryCA:             primaryCA,
		SecondaryCATotalCount: secondaryCACount,
	}

	return interfaceInfo, nil
}

func GetOrchestratorDetails() *OrchsestratorInfo {
	out, err := exec.Command("kubectl", "--version").Output()
	if err != nil {
		return nil
	}

	outStr := string(out)
	outStr = strings.TrimLeft(outStr, " ")
	resultArray := strings.Split(outStr, " ")
	orchestratorDetails := &OrchsestratorInfo{OrchestratorName: resultArray[0], OrchestratorVersion: resultArray[1]}
	return orchestratorDetails
}
