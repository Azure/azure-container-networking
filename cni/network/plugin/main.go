// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/network"
	"github.com/Azure/azure-container-networking/common"
	acn "github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/telemetry"
	"github.com/containernetworking/cni/pkg/skel"
)

const (
	hostNetAgentURL = "http://169.254.169.254/machine/plugins?comp=netagent&type=cnireport"
	ipamQueryURL    = "http://169.254.169.254/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
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
func markSendReport(reportManager *telemetry.ReportManager) {
	if err := reportManager.SetReportState(telemetry.CNITelemetryFile); err != nil {
		log.Printf("SetReportState failed due to %v", err)
		reflect.ValueOf(reportManager.Report).Elem().FieldByName("ErrorMessage").SetString(err.Error())

		if reportManager.SendReport() != nil {
			log.Printf("SendReport failed due to %v", err)
		}
	}
}

// send error report to hostnetagent if CNI encounters any error.
func reportPluginError(reportManager *telemetry.ReportManager, err error) {
	log.Printf("Report plugin error")
	reportManager.Report.(*telemetry.CNIReport).GetReport(pluginName, version, ipamQueryURL)
	reflect.ValueOf(reportManager.Report).Elem().FieldByName("ErrorMessage").SetString(err.Error())

	if err = reportManager.SendReport(); err != nil {
		log.Printf("SendReport failed due to %v", err)
	} else {
		markSendReport(reportManager)
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
			Context: "AzureCNI",
		},
	}

	reportManager.GetHostMetadata()
	reportManager.Report.(*telemetry.CNIReport).GetReport(pluginName, config.Version, ipamQueryURL)

	if !reportManager.GetReportState(telemetry.CNITelemetryFile) {
		log.Printf("GetReport state file didn't exist. Setting flag to true")

		err = reportManager.SendReport()
		if err != nil {
			log.Printf("SendReport failed due to %v", err)
		} else {
			markSendReport(reportManager)
		}
	}

	netPlugin, err := network.NewPlugin(&config)
	if err != nil {
		log.Printf("Failed to create network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
		os.Exit(1)
	}

	netPlugin.SetReportManager(reportManager)

	if err = netPlugin.Plugin.InitializeKeyValueStore(&config); err != nil {
		log.Printf("Failed to initialize key-value store of network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
		os.Exit(1)
	}

	defer func() {
		if errUninit := netPlugin.Plugin.UninitializeKeyValueStore(); errUninit != nil {
			log.Printf("Failed to uninitialize key-value store of network plugin, err:%v.\n", err)
		}

		if recover() != nil {
			os.Exit(1)
		}
	}()

	if err = netPlugin.Start(&config); err != nil {
		log.Printf("Failed to start network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
		panic("network plugin fatal error")
	}

	handled, err := handleIfCniUpdate(netPlugin.Update)
	if handled == true {
		log.Printf("CNI UPDATE finished.")
	} else if err = netPlugin.Execute(cni.PluginApi(netPlugin)); err != nil {
		log.Printf("Failed to execute network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
	}

	netPlugin.Stop()

	if err != nil {
		panic("network plugin fatal error")
	}

	// Report CNI successfully finished execution.
	reflect.ValueOf(reportManager.Report).Elem().FieldByName("CniSucceeded").SetBool(true)

	if err = reportManager.SendReport(); err != nil {
		log.Printf("SendReport failed due to %v", err)
	} else {
		markSendReport(reportManager)
	}
}
