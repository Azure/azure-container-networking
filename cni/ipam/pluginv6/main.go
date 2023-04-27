// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"fmt"
	"github.com/Azure/azure-container-networking/cni/log"
	"go.uber.org/zap/zapcore"
	"os"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/ipam"
	"github.com/Azure/azure-container-networking/common"
)

const (
	name = "azure-vnet-ipamv6"
)

// Version is populated by make during build.
var version string

// Main is the entry point for CNI IPAM plugin.
func main() {
	var config common.PluginConfig
	config.Version = version

	loggerCfg := &log.Config{
		Level:       zapcore.DebugLevel,
		LogPath:     log.LogPath + "azure-ipam.log",
		MaxSizeInMB: 5,
		MaxBackups:  8,
		Name:        name,
	}
	cleanup, err := log.New(loggerCfg)
	if err != nil {
		fmt.Printf("Failed to setup cni logging: %v\n", err)
		return
	}
	defer cleanup()

	ipamPlugin, err := ipam.NewPlugin(name, &config)
	if err != nil {
		fmt.Printf("Failed to create IPAM plugin, err:%v.\n", err)
		os.Exit(1)
	}

	if err := ipamPlugin.Plugin.InitializeKeyValueStore(&config); err != nil {
		fmt.Printf("Failed to initialize key-value store of ipam plugin, err:%v.\n", err)
		os.Exit(1)
	}

	defer func() {
		if errUninit := ipamPlugin.Plugin.UninitializeKeyValueStore(); errUninit != nil {
			fmt.Printf("Failed to uninitialize key-value store of ipam plugin, err:%v.\n", errUninit)
		}

		if recover() != nil {
			os.Exit(1)
		}
	}()

	err = ipamPlugin.Start(&config)
	if err != nil {
		fmt.Printf("Failed to start IPAM plugin, err:%v.\n", err)
		panic("ipam plugin fatal error")
	}

	err = ipamPlugin.Execute(cni.PluginApi(ipamPlugin))

	ipamPlugin.Stop()

	if err != nil {
		panic("ipam plugin fatal error")
	}
}
