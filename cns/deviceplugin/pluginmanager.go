package deviceplugin

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

var ErrUnknownDeviceType = errors.New("unknown device type")

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
	VnetNICPlugin *Plugin
	IBNICPlugin   *Plugin
	options       pluginManagerOptions
}

func NewPluginManager(l *zap.Logger, initialVnetNICCount, initialIBNICCount int, opts ...pluginManagerOption) *PluginManager {
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
		Logger: logger,
		VnetNICPlugin: NewPlugin(logger, string(v1alpha1.DeviceTypeVnetNIC), socketWatcher,
			getSocketName(options.devicePluginDirectory, v1alpha1.DeviceTypeVnetNIC), initialVnetNICCount,
			options.kubeletSocket, options.deviceCheckInterval),
		IBNICPlugin: NewPlugin(logger, string(v1alpha1.DeviceTypeInfiniBandNIC), socketWatcher,
			getSocketName(options.devicePluginDirectory, v1alpha1.DeviceTypeInfiniBandNIC), initialIBNICCount,
			options.kubeletSocket, options.deviceCheckInterval),
		options: options,
	}
}

// Run runs the plugin manager until the context is cancelled or error encountered
func (p *PluginManager) Run(ctx context.Context) error {
	// clean up any leftover state from previous failed plugins
	// this can happen if the process crashes before it is able to clean up after itself
	if err := p.cleanOldState(); err != nil {
		return errors.Wrap(err, "error cleaning state from previous plugin process")
	}

	var wg sync.WaitGroup
	wg.Add(1) //nolint:gomnd // starting two goroutines to wait on
	go func() {
		defer wg.Done()
		p.VnetNICPlugin.Run(ctx)
	}()

	wg.Add(1) //nolint:gomnd // starting two goroutines to wait on
	go func() {
		defer wg.Done()
		p.IBNICPlugin.Run(ctx)
	}()

	wg.Wait()
	return nil
}

func (p *PluginManager) TrackDevices(deviceType v1alpha1.DeviceType, count int) error {
	switch deviceType {
	case v1alpha1.DeviceTypeInfiniBandNIC:
		p.IBNICPlugin.UpdateDeviceCount(count)
	case v1alpha1.DeviceTypeVnetNIC:
		p.VnetNICPlugin.UpdateDeviceCount(count)
	default:
		return ErrUnknownDeviceType
	}

	return nil
}

func (p *PluginManager) cleanOldState() error {
	entries, err := os.ReadDir(p.options.devicePluginDirectory)
	if err != nil {
		return errors.Wrap(err, "error listing existing device plugin sockets")
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), path.Base(getSocketPrefix(p.options.devicePluginDirectory, v1alpha1.DeviceTypeVnetNIC))) ||
			strings.HasPrefix(entry.Name(), path.Base(getSocketPrefix(p.options.devicePluginDirectory, v1alpha1.DeviceTypeInfiniBandNIC))) {
			// try to delete it
			f := path.Join(p.options.devicePluginDirectory, entry.Name())
			if err := os.Remove(f); err != nil {
				return errors.Wrapf(err, "error removing old socket %q", f)
			}
		}
	}
	return nil
}

// getSocketPrefix returns a fully qualified path prefix for a given device type. For example, if the device plugin directory is
// /home/foo and the device type is acn.azure.com/vnet-nic, this function returns /home/foo/acn.azure.com_vnet-nic
func getSocketPrefix(devicePluginDirectory string, deviceType v1alpha1.DeviceType) string {
	sanitizedDeviceName := strings.ReplaceAll(string(deviceType), "/", "_")
	return path.Join(devicePluginDirectory, sanitizedDeviceName)
}

func getSocketName(devicePluginDirectory string, deviceType v1alpha1.DeviceType) string {
	return fmt.Sprintf("%s-%d.sock", getSocketPrefix(devicePluginDirectory, deviceType), time.Now().Unix())
}
