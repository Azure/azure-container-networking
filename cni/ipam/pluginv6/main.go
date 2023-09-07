// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/cni/ipam"
	zaplog "github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"go.uber.org/zap/zapcore"
)

const (
	name               = "azure-vnet-ipamv6"
	maxLogFileSizeInMb = 5
	maxLogFileCount    = 8
	component          = "cni"
)

// Version is populated by make during build.
var version string

// Main is the entry point for CNI IPAM plugin.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	var config common.PluginConfig
	config.Version = version

	logDirectory := "" // Sets the current location as log directory

	log.SetName(name)
	log.SetLevel(log.LevelInfo)
	if err := log.SetTargetLogDirectory(log.TargetLogfile, logDirectory); err != nil {
		fmt.Printf("Failed to setup cni logging: %v\n", err)
		return
	}

	defer log.Close()

	loggerCfg := &zaplog.Config{
		Level:       zapcore.DebugLevel,
		LogPath:     zaplog.LogPath + "azure-ipam.log",
		MaxSizeInMB: maxLogFileSizeInMb,
		MaxBackups:  maxLogFileCount,
		Name:        name,
		Component:   component,
	}
	zaplog.Initialize(ctx, loggerCfg)

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
	cancel()

	if err != nil {
		panic("ipam plugin fatal error")
	}
}
