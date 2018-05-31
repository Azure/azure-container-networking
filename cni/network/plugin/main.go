// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"os"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/network"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/telemetry"
)

const (
	hostNetAgentURL = "http://169.254.169.254/machine/plugins?comp=netagent&type=cnireport"
	ipamQueryURL    = "http://169.254.169.254/machine/plugins?comp=nmagent&type=getinterfaceinfov1"
	pluginName      = "CNI"
	reportType      = "application/json"
)

// Version is populated by make during build.
var version string

// If report write succeeded, mark the report flag state to false.
func markSendReport(reportManager *telemetry.ReportManager) {
	if err := reportManager.Report.SetReportState(); err != nil {
		log.Printf("SetReportState failed due to %v", err)
		reportManager.Report.ErrorMessage = err.Error()

		if reportManager.SendReport() != nil {
			log.Printf("SendReport failed due to %v", err)
		}
	}
}

// send error report to hostnetagent if CNI encounters any error.
func reportPluginError(reportManager *telemetry.ReportManager, err error) {
	log.Printf("Report plugin error")
	reportManager.GetReport(pluginName, version)
	reportManager.Report.ErrorMessage = err.Error()

	if err = reportManager.SendReport(); err != nil {
		log.Printf("SendReport failed due to %v", err)
	} else {
		markSendReport(reportManager)
	}
}

// Main is the entry point for CNI network plugin.
func main() {
	var config common.PluginConfig
	var err error
	config.Version = version
	reportManager := &telemetry.ReportManager{
		HostNetAgentURL: hostNetAgentURL,
		IpamQueryURL:    ipamQueryURL,
		ReportType:      reportType,
		Report:          &telemetry.Report{},
	}

	reportManager.GetReport(pluginName, config.Version)
	reportManager.Report.Context = "AzureCNI"

	if !reportManager.Report.GetReportState() {
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

	defer func() {
		if errUninit := netPlugin.Plugin.UninitializeKeyValueStore(); errUninit != nil {
			log.Printf("Failed to uninitialize key-value store of network plugin, err:%v.\n", err)
		}

		if recover() != nil {
			os.Exit(1)
		}
	}()

	if err = netPlugin.Plugin.InitializeKeyValueStore(&config); err != nil {
		log.Printf("Failed to initialize key-value store of network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
		panic("network plugin fatal error")
	}

	if err = netPlugin.Start(&config); err != nil {
		log.Printf("Failed to start network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
		panic("network plugin fatal error")
	}

	if err = netPlugin.Execute(cni.PluginApi(netPlugin)); err != nil {
		log.Printf("Failed to execute network plugin, err:%v.\n", err)
		reportPluginError(reportManager, err)
	}

	netPlugin.Stop()

	if err != nil {
		panic("network plugin fatal error")
	}

	// Report CNI successfully finished execution.
	reportManager.Report.CniSucceeded = true
	if err = reportManager.SendReport(); err != nil {
		log.Printf("SendReport failed due to %v", err)
	} else {
		markSendReport(reportManager)
	}
}
