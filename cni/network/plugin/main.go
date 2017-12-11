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

// Main is the entry point for CNI network plugin.
func main() {
	var config common.PluginConfig
	var err error
	config.Version = version

	reportManager := &telemetry.ReportManager{HostNetAgentURL: hostNetAgentURL, IpamQueryURL: ipamQueryURL, ReportType: reportType}

	reportManager.Report, err = reportManager.GetReport(pluginName, config.Version)
	if err != nil {
		log.Printf("GetReport failed due to %v", err)
		reportManager.Report.ErrorMessage = err.Error()
	}

	report := reportManager.Report

	if !report.GetReportState() {
		log.Printf("GetReport state file didn't exist. Setting flag to true")

		report.Context = "AzureCNI"

		err = reportManager.SendReport()
		if err != nil {
			log.Printf("SendReport failed due to %v", err)
		} else {
			if err = report.SetReportState(); err != nil {
				log.Printf("SetReportState failed due to %v", err)

				report.ErrorMessage = err.Error()
				if reportManager.SendReport() != nil {
					log.Printf("SendReport failed due to %v", err)
				}
			}
		}
	}

	netPlugin, err := network.NewPlugin(&config)
	if err != nil {
		log.Printf("Failed to create network plugin, err:%v.\n", err)

		report.ErrorMessage = err.Error()
		if err = reportManager.SendReport(); err != nil {
			log.Printf("SendReport failed due to %v", err)
		}

		os.Exit(1)
	}

	netPlugin.SetReportManager(reportManager)

	err = netPlugin.Start(&config)
	if err != nil {
		log.Printf("Failed to start network plugin, err:%v.\n", err)

		report.ErrorMessage = err.Error()
		if err = reportManager.SendReport(); err != nil {
			log.Printf("SendReport failed due to %v", err)
		}

		os.Exit(1)
	}

	err = netPlugin.Execute(cni.PluginApi(netPlugin))
	if err != nil {
		report.ErrorMessage = err.Error()
		if err = reportManager.SendReport(); err != nil {
			log.Printf("SendReport failed due to %v", err)
		}
	}

	netPlugin.Stop()

	if err != nil {
		os.Exit(1)
	}
}
