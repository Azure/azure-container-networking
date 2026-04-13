package deviceplugin

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const devicePrefix = "NIC-"

type deviceCounter interface {
	getDeviceCount() int
}

type Server struct {
	// NEW: embed the unimplemented server to satisfy the interface in newer gRPC/proto
	v1beta1.UnimplementedDevicePluginServer

	address             string
	logger              *zap.Logger
	deviceCounter       deviceCounter
	shutdownCh          <-chan struct{}
	deviceCheckInterval time.Duration
}

func NewServer(logger *zap.Logger, address string, deviceCounter deviceCounter, deviceCheckInterval time.Duration) *Server {
	return &Server{
		address:             address,
		logger:              logger,
		deviceCounter:       deviceCounter,
		deviceCheckInterval: deviceCheckInterval,
	}
}

// Run starts the grpc server and blocks until an error or context is cancelled. Wait on Ready to know when the server is ready.
func (s *Server) Run(ctx context.Context) error {
	grpcServer := grpc.NewServer()
	v1beta1.RegisterDevicePluginServer(grpcServer, s)

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.shutdownCh = childCtx.Done()

	// remove the socket if it already exists
	if err := os.Remove(s.address); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "error removing socket")
	}

	l, err := net.Listen("unix", s.address)
	if err != nil {
		return errors.Wrap(err, "error listening on socket")
	}
	defer l.Close()

	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(l); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return errors.Wrap(err, "error running grpc server")
	}
	return nil
}

// Ready blocks until the server is ready
func (s *Server) Ready(ctx context.Context) error {
	c, err := grpc.DialContext(ctx, s.address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock(), //nolint:staticcheck // TODO: Move to grpc.NewClient method
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			conn, err := (&net.Dialer{}).DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}),
	)
	if err != nil {
		return errors.Wrap(err, "error dialing local grpc server")
	}
	if err := c.Close(); err != nil {
		return errors.Wrap(err, "error closing connection to local grpc server")
	}
	return nil
}

// This is a dummy implementation for allocate to conform to the interface requirements.
// Allocate is called during container creation so that the Device
// Plugin can run device specific operations and instruct Kubelet
// of the steps to make the Device available in the container
// We are not using this functionality currently
func (s *Server) Allocate(_ context.Context, req *v1beta1.AllocateRequest) (*v1beta1.AllocateResponse, error) {
	s.logger.Info("allocate request", zap.Any("req", *req))
	crs := req.GetContainerRequests()
	resps := make([]*v1beta1.ContainerAllocateResponse, len(crs))
	for i, containerReq := range crs {
		resp := &v1beta1.ContainerAllocateResponse{
			Envs: make(map[string]string),
		}
		for j, id := range containerReq.GetDevicesIds() {
			resp.Envs[fmt.Sprintf("%s%d", devicePrefix, j)] = id
		}
		resps[i] = resp
	}
	r := &v1beta1.AllocateResponse{
		ContainerResponses: resps,
	}
	return r, nil
}

func (s *Server) ListAndWatch(_ *v1beta1.Empty, stream v1beta1.DevicePlugin_ListAndWatchServer) error {
	// send the initial count right away
	advertisedCount := s.deviceCounter.getDeviceCount()
	devices := make([]*v1beta1.Device, advertisedCount)
	for i := range devices {
		devices[i] = &v1beta1.Device{
			ID:     fmt.Sprintf("%s%d", devicePrefix, i),
			Health: v1beta1.Healthy,
		}
	}
	if err := stream.Send(&v1beta1.ListAndWatchResponse{
		Devices: devices,
	}); err != nil {
		return errors.Wrap(err, "error sending listAndWatch response")
	}

	// every interval, check if the current count has changed from what we've previously sent, and if so, send the new count
	ticker := time.NewTicker(s.deviceCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdownCh:
			return nil
		case <-stream.Context().Done():
			return errors.Wrap(stream.Context().Err(), "client context done")
		case <-ticker.C:
			currentCount := s.deviceCounter.getDeviceCount()
			if currentCount == advertisedCount {
				continue
			}
			advertisedCount = currentCount
			devices := make([]*v1beta1.Device, advertisedCount)
			for i := range devices {
				devices[i] = &v1beta1.Device{
					ID:     fmt.Sprintf("%s%d", devicePrefix, i),
					Health: v1beta1.Healthy,
				}
			}
			if err := stream.Send(&v1beta1.ListAndWatchResponse{
				Devices: devices,
			}); err != nil {
				return errors.Wrap(err, "error sending listAndWatch response")
			}
		}
	}
}

func (s *Server) GetDevicePluginOptions(context.Context, *v1beta1.Empty) (*v1beta1.DevicePluginOptions, error) {
	return &v1beta1.DevicePluginOptions{}, nil
}

func (s *Server) GetPreferredAllocation(context.Context, *v1beta1.PreferredAllocationRequest) (*v1beta1.PreferredAllocationResponse, error) {
	return &v1beta1.PreferredAllocationResponse{}, nil
}

func (s *Server) PreStartContainer(context.Context, *v1beta1.PreStartContainerRequest) (*v1beta1.PreStartContainerResponse, error) {
	return &v1beta1.PreStartContainerResponse{}, nil
}
