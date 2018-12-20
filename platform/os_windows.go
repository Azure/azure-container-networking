// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package platform

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-container-networking/log"
)

const (

	// CNMRuntimePath is the path where CNM state files are stored.
	CNMRuntimePath = ""

	// CNIRuntimePath is the path where CNI state files are stored.
	CNIRuntimePath = ""

	// CNSRuntimePath is the path where CNS state files are stored.
	CNSRuntimePath = ""

	// NPMRuntimePath is the path where NPM state files are stored.
	NPMRuntimePath = ""

	// DNCRuntimePath is the path where DNC state files are stored.
	DNCRuntimePath = ""
)

// GetOSInfo returns OS version information.
func GetOSInfo() string {
	return "windows"
}

// GetLastRebootTime returns the last time the system rebooted.
func GetLastRebootTime() (time.Time, error) {
	var systemBootTime string
	out, err := exec.Command("cmd", "/c", "systeminfo").Output()
	if err != nil {
		log.Printf("Failed to query systeminfo, err: %v", err)
		return time.Time{}.UTC(), err
	}

	systemInfo := strings.Split(string(out), "\n")
	for _, systemProperty := range systemInfo {
		if strings.Contains(systemProperty, "Boot Time") {
			systemBootTime = strings.TrimSpace(strings.Split(systemProperty, "System Boot Time:")[1])
		}
	}

	if len(strings.TrimSpace(systemBootTime)) == 0 {
		log.Printf("Failed to retrieve boot time from systeminfo")
		return time.Time{}.UTC(), fmt.Errorf("Failed to retrieve boot time from systeminfo")
	}

	log.Printf("Boot time: %s", systemBootTime)
	// The System Boot Time is in the following format "01/02/2006, 03:04:05 PM"
	// Formulate the Boot Time in the format: "2006-01-02 15:04:05"
	bootDate := strings.Split(systemBootTime, " ")[0]
	bootTime := strings.Split(systemBootTime, " ")[1]
	bootPM := strings.Contains(strings.Split(systemBootTime, " ")[2], "PM")

	month := strings.Split(bootDate, "/")[0]
	day := strings.Split(bootDate, "/")[1]
	year := strings.Split(bootDate, "/")[2]
	year = strings.Trim(year, ",")
	hour := strings.Split(bootTime, ":")[0]
	hourInt, _ := strconv.Atoi(hour)
	min := strings.Split(bootTime, ":")[1]
	sec := strings.Split(bootTime, ":")[2]

	if bootPM && hourInt < 12 {
		hourInt += 12
	} else if !bootPM && hourInt == 12 {
		hourInt = 0
	}

	hour = strconv.Itoa(hourInt)
	systemBootTime = year + "-" + month + "-" + day + " " + hour + ":" + min + ":" + sec
	log.Printf("Formatted Boot time: %s", systemBootTime)

	// Parse the boot time.
	layout := "2006-01-02 15:04:05"
	rebootTime, err := time.ParseInLocation(layout, systemBootTime, time.Local)
	if err != nil {
		log.Printf("Failed to parse boot time, err:%v", err)
		return time.Time{}.UTC(), err
	}

	return rebootTime.UTC(), nil
}

func ExecuteCommand(command string) (string, error) {
	return "", nil
}

func SetOutboundSNAT(subnet string) error {
	return nil
}

// ClearNetworkConfiguration clears the azure-vnet.json contents.
// This will be called only when reboot is detected - This is windows specific
func ClearNetworkConfiguration() (bool, error) {
	jsonStore := CNIRuntimePath + "azure-vnet.json"
	log.Printf("Deleting the json store %s", jsonStore)
	cmd := exec.Command("cmd", "/c", "del", jsonStore)

	if err := cmd.Run(); err != nil {
		log.Printf("Error deleting the json store %s", jsonStore)
		return true, err
	}

	return true, nil
}
