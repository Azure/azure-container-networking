package main

import (
	"log"
	"os"

	"github.com/Azure/azure-container-networking/azure-ipam/internal/buildinfo"
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
	logger, cleanup, err := NewLogger(Env(buildinfo.BuildEnv))
	if err != nil {
		return errors.Wrapf(err, "failed to setup IPAM logging")
	}
	logger.Debug("logger construction succeeded")
	defer cleanup(logger) // nolint

	// Create IPAM plugin with logger and CNS client
	client, err := cnsclient.New(cnsBaseURL, csnReqTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize CNS client")
	}
	plugin, err := NewPlugin(logger, client)
	if err != nil {
		logger.Error("Failed to create IPAM plugin")
		return errors.Wrapf(err, "failed to create IPAM plugin")
	}

	return skel.PluginMainWithError(plugin.CmdAdd, plugin.CmdCheck, plugin.CmdDel, version.All, bv.BuildString(pluginName))
}
