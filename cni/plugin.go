// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package cni

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/Azure/azure-container-networking/store"
	cniInvoke "github.com/containernetworking/cni/pkg/invoke"
	cniSkel "github.com/containernetworking/cni/pkg/skel"
	cniTypes "github.com/containernetworking/cni/pkg/types"
	cniTypesCurr "github.com/containernetworking/cni/pkg/types/100"
	cniVers "github.com/containernetworking/cni/pkg/version"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var logger = log.CNILogger.With(zap.String("component", "cni-plugin"))
var storeLogger = log.CNILogger.With(zap.String("component", "cni-store"))

var errEmptyContent = errors.New("read content is zero bytes")

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

			logger.Info("Recovered panic",
				zap.String("error", cniErr.Msg),
				zap.String("details", cniErr.Details))
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

	logger.Info("Calling ADD", zap.String("plugin", pluginName))
	defer func() {
		logger.Info("Plugin returned",
			zap.String("plugin", pluginName),
			zap.Any("result", result),
			zap.Error(err))
	}()

	os.Setenv(Cmd, CmdAdd)

	res, err := cniInvoke.DelegateAdd(context.TODO(), pluginName, nwCfg.Serialize(), nil)
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

	logger.Info("Calling DEL",
		zap.String("plugin", pluginName),
		zap.Any("config", nwCfg))
	defer func() {
		logger.Info("Plugin returned",
			zap.String("plugin", pluginName),
			zap.Error(err))
	}()

	os.Setenv(Cmd, CmdDel)

	err = cniInvoke.DelegateDel(context.TODO(), pluginName, nwCfg.Serialize(), nil)
	if err != nil {
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

	logger.Error("error",
		zap.String("plugin", plugin.Name),
		zap.Error(cniErr))

	return cniErr
}

// Errorf creates and logs a custom CNI error according to a format specifier.
func (plugin *Plugin) Errorf(format string, args ...interface{}) *cniTypes.Error {
	return plugin.Error(fmt.Errorf(format, args...))
}

// RetriableError logs and returns a CNI error with the TryAgainLater error code
func (plugin *Plugin) RetriableError(err error) *cniTypes.Error {
	tryAgainErr := cniTypes.NewError(cniTypes.ErrTryAgainLater, err.Error(), "")
	logger.Error("retry failed",
		zap.String("name", plugin.Name),
		zap.String("error", tryAgainErr.Error()))
	return tryAgainErr
}

// Initialize key-value store
func (plugin *Plugin) InitializeKeyValueStore(config *common.PluginConfig) error {
	// Create the key value store.
	if plugin.Store == nil {
		lockclient, err := processlock.NewFileLock(platform.CNILockPath + plugin.Name + store.LockExtension)
		if err != nil {
			logger.Error("Error initializing file lock", zap.Error(err))
			return errors.Wrap(err, "error creating new filelock")
		}

		plugin.Store, err = store.NewJsonFileStore(platform.CNIRuntimePath+plugin.Name+".json", lockclient, storeLogger)
		if err != nil {
			logger.Error("Failed to create store", zap.Error(err))
			return err
		}
	}

	// Acquire store lock. For windows 1m timeout is used while for Linux 10s timeout is assigned.
	var lockTimeoutValue time.Duration = store.DefaultLockTimeoutLinux
	if runtime.GOOS == "windows" {
		lockTimeoutValue = store.DefaultLockTimeoutWindows
	}
	// Acquire store lock.
	if err := plugin.Store.Lock(lockTimeoutValue); err != nil {
		logger.Error("[cni] Failed to lock store", zap.Error(err))
		return errors.Wrap(err, "error Acquiring store lock")
	}

	config.Store = plugin.Store

	return nil
}

// Uninitialize key-value store
func (plugin *Plugin) UninitializeKeyValueStore() error {
	if plugin.Store != nil {
		err := plugin.Store.Unlock()
		if err != nil {
			logger.Error("Failed to unlock store", zap.Error(err))
			return err
		}
	}
	plugin.Store = nil

	return nil
}
