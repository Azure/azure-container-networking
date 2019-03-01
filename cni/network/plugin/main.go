// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"time"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/network"
	"github.com/Azure/azure-container-networking/common"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/containernetworking/cni/pkg/skel"
)

const (
	hostNetAgentURL = "http://168.63.129.16/machine/plugins?comp=netagent&type=cnireport"
	ipamQueryURL    = "http://168.63.129.16/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
	pluginName      = "CNI"
)

// Version is populated by make during build.
var version string

// Command line arguments for CNI plugin.
var args = acn.ArgumentList{
	{
		Name:         acn.OptVersion,
		Shorthand:    acn.OptVersionAlias,
		Description:  "Print version information",
		Type:         "bool",
		DefaultValue: false,
	},
}

// Prints version information.
func printVersion() {
	fmt.Printf("Azure CNI Version %v\n", version)
}

// If report write succeeded, mark the report flag state to false.
func markSendReport(reportManager *telemetry.ReportManager, tb *telemetry.TelemetryBuffer) {
	if err := reportManager.SetReportState(telemetry.CNITelemetryFile); err != nil {
		log.Printf("SetReportState failed due to %v", err)
		reflect.ValueOf(reportManager.Report).Elem().FieldByName("ErrorMessage").SetString(err.Error())

		if err := reportManager.SendReport(tb); err != nil {
			log.Printf("SendReport failed due to %v", err)
		}
	}
}

// send error report to hostnetagent if CNI encounters any error.
func reportPluginError(reportManager *telemetry.ReportManager, tb *telemetry.TelemetryBuffer, err error) {
	log.Printf("Report plugin error")
	reportManager.Report.(*telemetry.CNIReport).GetReport(pluginName, version, ipamQueryURL)
	reflect.ValueOf(reportManager.Report).Elem().FieldByName("ErrorMessage").SetString(err.Error())

	if err := reportManager.SendReport(tb); err != nil {
		log.Printf("SendReport failed due to %v", err)
	}
}

func validateConfig(jsonBytes []byte) error {
	var conf struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(jsonBytes, &conf); err != nil {
		return fmt.Errorf("error reading network config: %s", err)
	}
	if conf.Name == "" {
		return fmt.Errorf("missing network name")
	}
	return nil
}

func getCmdArgsFromEnv() (string, *skel.CmdArgs, error) {
	log.Printf("Going to read from stdin")
	stdinData, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return "", nil, fmt.Errorf("error reading from stdin: %v", err)
	}

	cmdArgs := &skel.CmdArgs{
		ContainerID: os.Getenv("CNI_CONTAINERID"),
		Netns:       os.Getenv("CNI_NETNS"),
		IfName:      os.Getenv("CNI_IFNAME"),
		Args:        os.Getenv("CNI_ARGS"),
		Path:        os.Getenv("CNI_PATH"),
		StdinData:   stdinData,
	}

	cmd := os.Getenv("CNI_COMMAND")
	return cmd, cmdArgs, nil
}

func handleIfCniUpdate(update func(*skel.CmdArgs) error) (bool, error) {
	isupdate := true

	if os.Getenv("CNI_COMMAND") != cni.CmdUpdate {
		return false, nil
	}

	log.Printf("CNI UPDATE received.")

	_, cmdArgs, err := getCmdArgsFromEnv()
	if err != nil {
		log.Printf("Received error while retrieving cmds from environment: %+v", err)
		return isupdate, err
	}

	log.Printf("Retrieved command args for update +%v", cmdArgs)
	err = validateConfig(cmdArgs.StdinData)
	if err != nil {
		log.Printf("Failed to handle CNI UPDATE, err:%v.", err)
		return isupdate, err
	}

	err = update(cmdArgs)
	if err != nil {
		log.Printf("Failed to handle CNI UPDATE, err:%v.", err)
		return isupdate, err
	}

	return isupdate, nil
}

// Main is the entry point for CNI network plugin.
func main() {

	// Initialize and parse command line arguments.
	acn.ParseArgs(&args, printVersion)
	vers := acn.GetArg(acn.OptVersion).(bool)

	if vers {
		printVersion()
		os.Exit(0)
	}

	var (
		config common.PluginConfig
		err    error
	)

	config.Version = version
	reportManager := &telemetry.ReportManager{
		HostNetAgentURL: hostNetAgentURL,
		ContentType:     telemetry.ContentType,
		Report: &telemetry.CNIReport{
			Context:          "AzureCNI",
			SystemDetails:    telemetry.SystemInfo{},
			InterfaceDetails: telemetry.InterfaceInfo{},
			BridgeDetails:    telemetry.BridgeInfo{},
		},
	}

	cniReport := reportManager.Report.(*telemetry.CNIReport)

	upTime, err := platform.GetLastRebootTime()
	if err == nil {
		cniReport.VMUptime = upTime.Format("2006-01-02 15:04:05")
	}

	tb := telemetry.NewTelemetryBuffer("")

	for attempt := 0; attempt < 2; attempt++ {
		err = tb.Connect()
		if err != nil {
			log.Printf("Connection to telemetry socket failed: %v", err)
			tb.Cleanup(telemetry.FdName)
			telemetry.StartTelemetryService()
		} else {
			tb.Connected = true
			log.Printf("Connected to telemetry service")
			break
		}
	}

	defer tb.Close()

	t := time.Now()
	cniReport.Timestamp = t.Format("2006-01-02 15:04:05")
	cniReport.GetReport(pluginName, version, ipamQueryURL)

	if !reportManager.GetReportState(telemetry.CNITelemetryFile) {
		log.Printf("GetReport state file didn't exist. Setting flag to true")

		err = reportManager.SendReport(tb)
		if err != nil {
			log.Printf("SendReport failed due to %v", err)
		} else {
			markSendReport(reportManager, tb)
		}
	}

	startTime := time.Now().UnixNano() / int64(time.Millisecond)

	netPlugin, err := network.NewPlugin(&config)
	if err != nil {
		log.Printf("Failed to create network plugin, err:%v.\n", err)
		reportPluginError(reportManager, tb, err)
		return
	}

	netPlugin.SetCNIReport(cniReport)

	if err = netPlugin.Plugin.InitializeKeyValueStore(&config); err != nil {
		log.Printf("Failed to initialize key-value store of network plugin, err:%v.\n", err)
		reportPluginError(reportManager, tb, err)
		return
	}

	defer func() {
		if errUninit := netPlugin.Plugin.UninitializeKeyValueStore(); errUninit != nil {
			log.Printf("Failed to uninitialize key-value store of network plugin, err:%v.\n", err)
		}

		if recover() != nil {
			return
		}
	}()

	if err = netPlugin.Start(&config); err != nil {
		log.Printf("Failed to start network plugin, err:%v.\n", err)
		reportPluginError(reportManager, tb, err)
		panic("network plugin start fatal error")
	}

	handled, err := handleIfCniUpdate(netPlugin.Update)
	if handled == true {
		log.Printf("CNI UPDATE finished.")
	} else if err = netPlugin.Execute(cni.PluginApi(netPlugin)); err != nil {
		log.Printf("Failed to execute network plugin, err:%v.\n", err)
	}

	endTime := time.Now().UnixNano() / int64(time.Millisecond)
	reflect.ValueOf(reportManager.Report).Elem().FieldByName("OperationDuration").SetInt(int64(endTime - startTime))

	netPlugin.Stop()

	if err != nil {
		reportPluginError(reportManager, tb, err)
		panic("network plugin execute fatal error")
	}

	// Report CNI successfully finished execution.
	reflect.ValueOf(reportManager.Report).Elem().FieldByName("CniSucceeded").SetBool(true)

	if err = reportManager.SendReport(tb); err != nil {
		log.Printf("SendReport failed due to %v", err)
	} else {
		log.Printf("Sending report succeeded")
		markSendReport(reportManager, tb)
	}
}
