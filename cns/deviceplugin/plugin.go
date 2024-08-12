package deviceplugin

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type Plugin struct {
	Logger              *zap.Logger
	ResourceName        string
	SocketWatcher       *SocketWatcher
	Socket              string
	deviceCountMutex    sync.Mutex
	deviceCount         int
	kubeletSocket       string
	deviceCheckInterval time.Duration
}

func NewPlugin(l *zap.Logger, resourceName string, socketWatcher *SocketWatcher, socket string,
	initialDeviceCount int, kubeletSocket string, deviceCheckInterval time.Duration,
) *Plugin {
	return &Plugin{
		Logger:              l.With(zap.String("resourceName", resourceName)),
		ResourceName:        resourceName,
		SocketWatcher:       socketWatcher,
		Socket:              socket,
		deviceCount:         initialDeviceCount,
		kubeletSocket:       kubeletSocket,
		deviceCheckInterval: deviceCheckInterval,
	}
}

// Run runs the plugin until the context is cancelled, restarting the server as needed
func (p *Plugin) Run(ctx context.Context) {
	defer p.mustCleanUp()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			p.Logger.Info("starting device plugin for resource", zap.String("resource", p.ResourceName))
			if err := p.run(ctx); err != nil {
				p.Logger.Error("device plugin for resource exited", zap.String("resource", p.ResourceName), zap.Error(err))
			}
		}
	}
}

func (p *Plugin) run(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s := NewServer(p.Logger, p.Socket, p, p.deviceCheckInterval)
	runErrChan := make(chan error, 2) //nolint:gomnd // disabled in favor of readability
	go func(errChan chan error) {
		if err := s.Run(childCtx); err != nil {
			errChan <- err
		}
	}(runErrChan)

	// wait till the server is ready before registering with kubelet
	readyErrChan := make(chan error, 2) //nolint:gomnd // disabled in favor of readability
	go func(errChan chan error) {
		errChan <- s.Ready(childCtx)
	}(readyErrChan)

	select {
	case err := <-runErrChan:
		return errors.Wrap(err, "error starting grpc server")
	case err := <-readyErrChan:
		if err != nil {
			return errors.Wrap(err, "error waiting on grpc server to be ready")
		}
	case <-ctx.Done():
		return nil
	}

	p.Logger.Info("registering with kubelet")
	// register with kubelet
	if err := p.registerWithKubelet(childCtx); err != nil {
		return errors.Wrap(err, "failed to register with kubelet")
	}

	// run until the socket goes away or the context is cancelled
	<-p.SocketWatcher.WatchSocket(childCtx, p.Socket)
	return nil
}

func (p *Plugin) registerWithKubelet(ctx context.Context) error {
	conn, err := grpc.Dial(p.kubeletSocket, grpc.WithTransportCredentials(insecure.NewCredentials()), //nolint:staticcheck // TODO: Move to grpc.NewClient method
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			conn, err := d.DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}))
	if err != nil {
		return errors.Wrap(err, "error connecting to kubelet")
	}
	defer conn.Close()

	client := v1beta1.NewRegistrationClient(conn)
	request := &v1beta1.RegisterRequest{
		Version:      v1beta1.Version,
		Endpoint:     filepath.Base(p.Socket),
		ResourceName: p.ResourceName,
	}
	if _, err = client.Register(ctx, request); err != nil {
		return errors.Wrap(err, "error sending request to register with kubelet")
	}
	return nil
}

func (p *Plugin) mustCleanUp() {
	p.Logger.Info("cleaning up device plugin")
	if err := os.Remove(p.Socket); err != nil && !os.IsNotExist(err) {
		p.Logger.Panic("failed to remove socket", zap.Error(err))
	}
}

func (p *Plugin) UpdateDeviceCount(count int) {
	p.deviceCountMutex.Lock()
	p.deviceCount = count
	p.deviceCountMutex.Unlock()
}

func (p *Plugin) getDeviceCount() int {
	p.deviceCountMutex.Lock()
	defer p.deviceCountMutex.Unlock()
	return p.deviceCount
}
