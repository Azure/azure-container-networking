// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cni

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/platform"

	cniInvoke "github.com/containernetworking/cni/pkg/invoke"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/current"
	cniVers "github.com/containernetworking/cni/pkg/version"
	bolt "go.etcd.io/bbolt"
)

// Plugin is the parent class for CNI plugins.
type Plugin struct {
	*common.Plugin
	version string
}

// NewPlugin creates a new CNI plugin.
func NewPlugin(name, version string) (*Plugin, error) {
	// Setup base plugin.
	plugin, err := common.NewPlugin(name, version)
	if err != nil {
		return nil, err
	}

	return &Plugin{
		Plugin:  plugin,
		version: version,
	}, nil
}

// Initialize initializes the plugin.
func (plugin *Plugin) Initialize(config *common.PluginConfig) error {
	// Initialize the base plugin.
	plugin.Plugin.Initialize(config)

	// Initialize logging.
	log.SetName(plugin.Name)
	log.SetLevel(log.LevelInfo)
	err := log.SetTarget(log.TargetLogfile)
	if err != nil {
		log.Printf("[cni] Failed to configure logging, err:%v.\n", err)
		return err
	}
	return nil
}

// Uninitialize uninitializes the plugin.
func (plugin *Plugin) Uninitialize() {
	plugin.Plugin.Uninitialize()
}

// Execute executes the CNI command.
func (plugin *Plugin) Execute(api PluginApi) (err error) {
	// Recover from panics and convert them to CNI errors.
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 1<<12)
			len := runtime.Stack(buf, false)

			cniErr := &cniTypes.Error{
				Code:    ErrRuntime,
				Msg:     fmt.Sprintf("%v", r),
				Details: string(buf[:len]),
			}
			cniErr.Print()
			err = cniErr

			log.Printf("[cni] Recovered panic: %v %v\n", cniErr.Msg, cniErr.Details)
		}
	}()

	// Set supported CNI versions.
	pluginInfo := cniVers.PluginSupports(supportedVersions...)

	// Parse args and call the appropriate cmd handler.
	cniErr := cniSkel.PluginMainWithError(api.Add, api.Get, api.Delete, pluginInfo, plugin.version)
	if cniErr != nil {
		cniErr.Print()
		return cniErr
	}

	return nil
}

// DelegateAdd calls the given plugin's ADD command and returns the result.
func (plugin *Plugin) DelegateAdd(pluginName string, nwCfg *NetworkConfig) (*cniTypesCurr.Result, error) {
	var result *cniTypesCurr.Result
	var err error

	log.Printf("[cni] Calling plugin %v ADD nwCfg:%+v.", pluginName, nwCfg)
	defer func() { log.Printf("[cni] Plugin %v returned result:%+v, err:%v.", pluginName, result, err) }()

	os.Setenv(Cmd, CmdAdd)

	res, err := cniInvoke.DelegateAdd(pluginName, nwCfg.Serialize(), nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to delegate: %v", err)
	}

	result, err = cniTypesCurr.NewResultFromResult(res)
	if err != nil {
		return nil, fmt.Errorf("Failed to convert result: %v", err)
	}

	return result, nil
}

// DelegateDel calls the given plugin's DEL command and returns the result.
func (plugin *Plugin) DelegateDel(pluginName string, nwCfg *NetworkConfig) error {
	var err error

	log.Printf("[cni] Calling plugin %v DEL nwCfg:%+v.", pluginName, nwCfg)
	defer func() { log.Printf("[cni] Plugin %v returned err:%v.", pluginName, err) }()

	os.Setenv(Cmd, CmdDel)

	if err := cniInvoke.DelegateDel(pluginName, nwCfg.Serialize(), nil); err != nil {
		return fmt.Errorf("Failed to delegate: %v", err)
	}

	return nil
}

// Error creates and logs a structured CNI error.
func (plugin *Plugin) Error(err error) *cniTypes.Error {
	var cniErr *cniTypes.Error
	var ok bool

	// Wrap error if necessary.
	if cniErr, ok = err.(*cniTypes.Error); !ok {
		cniErr = &cniTypes.Error{Code: 100, Msg: err.Error()}
	}

	log.Printf("[%v] %+v.", plugin.Name, cniErr.Error())

	return cniErr
}

// Errorf creates and logs a custom CNI error according to a format specifier.
func (plugin *Plugin) Errorf(format string, args ...interface{}) *cniTypes.Error {
	return plugin.Error(fmt.Errorf(format, args...))
}

// Opens the database
func (plugin *Plugin) OpenDatabase(config *common.PluginConfig) error {
	// Create the database
	if plugin.Database == nil {
		var err error
		plugin.Database, err = bolt.Open(filepath.Join(platform.CNIRuntimePath, plugin.Name)+".db", 0600, nil)
		if err != nil {
			log.Printf("[cni] Failed to create database: %v.", err)
			return err
		}
	}

	config.Database = plugin.Database

	return nil
}

// Closes the database
func (plugin *Plugin) CloseDatabase() error {
	if plugin.Database != nil {
		plugin.Database.Close()
	}
	plugin.Database = nil

	return nil
}
