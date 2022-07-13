package main

import (
	"log"
	"os"

	"github.com/Azure/azure-container-networking/azure-ipam/logger"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/pkg/errors"
)

func main() {
	if err := executePlugin(); err != nil {
		log.Printf("error executing azure-ipam plugin: %v\n", err)
		os.Exit(1)
	}
}

func executePlugin() error {
	// logger config
	var loggerCfg *logger.Config
	loggerCfg.Level = "debug"
	loggerCfg.OutputPaths = "stdout"
	loggerCfg.ErrorOutputPaths = "stderr"

	// Create logger
	pluginLogger, cleanup, err := logger.New(loggerCfg)
	if err != nil {
		return errors.Wrapf(err, "failed to setup IPAM logging")
	}
	pluginLogger.Debug("logger construction succeeded")
	defer cleanup()

	// Create CNS client
	client, err := cnsclient.New(cnsBaseURL, cnsReqTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize CNS client")
	}

	// Create IPAM plugin
	plugin, err := NewPlugin(pluginLogger, client, os.Stdout)
	if err != nil {
		pluginLogger.Error("Failed to create IPAM plugin")
		return errors.Wrapf(err, "failed to create IPAM plugin")
	}

	// Execute CNI plugin
	return skel.PluginMainWithError(plugin.CmdAdd, plugin.CmdCheck, plugin.CmdDel, version.All, bv.BuildString(pluginName))
}
