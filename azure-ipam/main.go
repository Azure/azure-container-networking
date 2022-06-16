package main

import (
	"encoding/json"
	"log"
	"os"

	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

func main() {
	if err := executePlugin(); err != nil {
		log.Printf("error executing azure-ipam plugin: %v\n", err)
		os.Exit(1)
	}
}

func executePlugin() error {
	zapConfig := []byte(`{
		"level": "debug",
		"encoding": "json",
		"outputPaths": ["/tmp/logs"],
		"errorOutputPaths": ["stderr"],
		"encoderConfig": {
		  "messageKey": "msg",
		  "levelKey": "level",
		  "levelEncoder": "lowercase"
		}
	  }`)

	var cfg zap.Config
	if err := json.Unmarshal(zapConfig, &cfg); err != nil {
		return errors.Wrapf(err, "failed to unmarshal zap config")
	}

	logger, err := cfg.Build()
	defer logger.Sync() // nolint
	if err != nil {
		return errors.Wrapf(err, "failed to setup IPAM logging")
	}
	logger.Info("logger construction succeeded")

	// Create IPAM plugin with logger and CNS client
	client, err := cnsclient.New(cnsBaseURL, csnReqTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize CNS client")
	}
	plugin, err := NewPlugin(logger, client)
	if err != nil {
		logger.Info("Failed to create IPAM plugin")
		return errors.Wrapf(err, "failed to create IPAM plugin")
	}

	return skel.PluginMainWithError(plugin.CmdAdd, plugin.CmdCheck, plugin.CmdDel, version.All, bv.BuildString(pluginName))
}
