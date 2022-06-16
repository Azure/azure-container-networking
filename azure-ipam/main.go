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
		log.Printf("[%s] Error executing CNS IPAM plugin: %s\n", PLUGIN_NAME, err)
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
	defer logger.Sync()
	if err != nil {
		return errors.Wrapf(err, "failed to setup IPAM logging")
	}
	logger.Info("logger construction succeeded")

	// Create IPAM plugin with logger and CNS client
	client, err := cnsclient.New(CNS_BASE_URL, CNS_REQ_TIMEOUT)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize CNS client")
	}
	plugin, err := NewPlugin(logger, client)
	if err != nil {
		logger.Info("Failed to create IPAM plugin")
		return errors.Wrapf(err, "[%s] Failed to create IPAM plugin", PLUGIN_NAME)
	}

	return skel.PluginMainWithError(plugin.CmdAdd, plugin.CmdCheck, plugin.CmdDel, version.All, bv.BuildString(PLUGIN_NAME))
}
