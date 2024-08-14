package deviceplugin

import (
	"context"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	defaultDevicePluginDirectory = "/var/lib/kubelet/device-plugins"
	defaultDeviceCheckInterval   = 5 * time.Second
)

type pluginManagerOptions struct {
	devicePluginDirectory string
	kubeletSocket         string
	deviceCheckInterval   time.Duration
}

type pluginManagerOption func(*pluginManagerOptions)

func PluginManagerSocketPrefix(prefix string) func(*pluginManagerOptions) {
	return func(opts *pluginManagerOptions) {
		opts.devicePluginDirectory = prefix
	}
}

func PluginManagerKubeletSocket(socket string) func(*pluginManagerOptions) {
	return func(opts *pluginManagerOptions) {
		opts.kubeletSocket = socket
	}
}

func PluginDeviceCheckInterval(i time.Duration) func(*pluginManagerOptions) {
	return func(opts *pluginManagerOptions) {
		opts.deviceCheckInterval = i
	}
}

// PluginManager runs device plugins for vnet nics and ib nics
type PluginManager struct {
	Logger        *zap.Logger
	plugins       []*Plugin
	socketWatcher *SocketWatcher
	options       pluginManagerOptions
	mu            sync.Mutex
}

func NewPluginManager(l *zap.Logger, opts ...pluginManagerOption) *PluginManager {
	logger := l.With(zap.String("component", "devicePlugin"))
	socketWatcher := NewSocketWatcher(logger)
	options := pluginManagerOptions{
		devicePluginDirectory: defaultDevicePluginDirectory,
		kubeletSocket:         v1beta1.KubeletSocket,
		deviceCheckInterval:   defaultDeviceCheckInterval,
	}
	for _, o := range opts {
		o(&options)
	}
	return &PluginManager{
		Logger:        logger,
		socketWatcher: socketWatcher,
		options:       options,
	}
}

func (pm *PluginManager) AddPlugin(deviceType v1alpha1.DeviceType, deviceCount int) *PluginManager {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	p := NewPlugin(pm.Logger, string(deviceType), pm.socketWatcher,
		pm.options.devicePluginDirectory, deviceCount, deviceType, pm.options.kubeletSocket, pm.options.deviceCheckInterval)
	pm.plugins = append(pm.plugins, p)
	return pm
}

// Run runs the plugin manager until the context is cancelled or error encountered
func (pm *PluginManager) Run(ctx context.Context) error {
	// clean up any leftover state from previous failed plugins
	// this can happen if the process crashes before it is able to clean up after itself
	for _, plugin := range pm.plugins {
		if err := plugin.CleanOldState(); err != nil {
			return errors.Wrap(err, "error cleaning state from previous plugin process")
		}
	}

	var wg sync.WaitGroup
	for _, plugin := range pm.plugins {
		wg.Add(1) //nolint:gomnd // in favor of readability
		go func(p *Plugin) {
			defer wg.Done()
			p.Run(ctx)
		}(plugin)
	}

	wg.Wait()
	return nil
}

func (pm *PluginManager) TrackDevices(deviceType v1alpha1.DeviceType, count int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, plugin := range pm.plugins {
		if plugin.deviceType == deviceType {
			plugin.UpdateDeviceCount(count)
			break
		}
	}
}
