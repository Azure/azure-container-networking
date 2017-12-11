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
	"strconv"
	"strings"
	"syscall"

	"github.com/Azure/azure-container-networking/common"
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
	TelemetryFile = "/var/run/AzureCNITelemetry.json"
	MB            = 1048576
	KB            = 1024
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

// This function retrieves VMs memory usage.
func getMemInfo() (*MemInfo, error) {
	var (
		memArray []uint64
	)

	linesArr, err := ReadFileByLines("/proc/meminfo")
	if err != nil {
		return nil, err
	}

	for _, line := range linesArr {
		fieldArray := strings.Split(line, ":")
		fieldArray[0] = strings.TrimSpace(fieldArray[0])
		if strings.Compare(fieldArray[0], "MemTotal") == 0 {
			memTotalStr := strings.TrimLeft(fieldArray[1], " ")
			memTotalArr := strings.Split(memTotalStr, " ")
			memTotal, err := strconv.ParseUint(memTotalArr[0], 0, 64)
			if err != nil {
				return nil, fmt.Errorf("Failed in atoi conversion of memtotal %v", err)
			}
			memTotal = memTotal / KB
			memArray = append(memArray, memTotal)
		} else if strings.Compare(fieldArray[0], "MemFree") == 0 {
			memFreeStr := strings.TrimLeft(fieldArray[1], " ")
			memFreeArr := strings.Split(memFreeStr, " ")
			memFree, err := strconv.ParseUint(memFreeArr[0], 0, 64)
			if err != nil {
				return nil, fmt.Errorf("Failed in atoi conversion of memtotal %v", err)
			}

			memFree = memFree / KB
			memArray = append(memArray, memFree)
		}
	}

	memInfo := &MemInfo{MemTotal: memArray[0], MemFree: memArray[1]}

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
func GetSystemDetails() (*SystemInfo, error) {
	var cpuCount int = 0

	linesArr, err := ReadFileByLines("/proc/cpuinfo")
	if err != nil {
		return nil, err
	}

	for _, line := range linesArr {
		fieldArray := strings.Split(line, ":")
		fieldArray[0] = strings.TrimSpace(fieldArray[0])
		if strings.Compare(fieldArray[0], "processor") == 0 {
			cpuCount += 1
		}
	}

	memInfo, err := getMemInfo()
	if err != nil {
		return nil, err
	}

	diskInfo, err := getDiskInfo("/")
	if err != nil {
		return nil, err
	}

	sysInfo := &SystemInfo{
		MemVMTotal:  memInfo.MemTotal,
		MemVMFree:   memInfo.MemFree,
		DiskVMTotal: diskInfo.DiskTotal,
		DiskVMFree:  diskInfo.DiskFree,
		CPUCount:    cpuCount,
	}

	return sysInfo, nil
}

// This function  creates a report with os details(ostype, version).
func GetOSDetails() (*OSInfo, error) {
	osType := "Linux"

	linesArr, err := ReadFileByLines("/etc/issue")
	if err != nil {
		return nil, err
	}

	osInfoArr := strings.Split(linesArr[0], " ")

	out, err := exec.Command("uname", "-r").Output()
	kernelVersion := string(out)

	osDetails := &OSInfo{OSType: osType, OSVersion: osInfoArr[1], KernelVersion: kernelVersion, OSDistribution: osInfoArr[0]}
	return osDetails, nil
}

// This function  creates a report with interface details(ip, mac, name, secondaryca count).
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

// This function  creates a report with orchestrator details(name, version).
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
