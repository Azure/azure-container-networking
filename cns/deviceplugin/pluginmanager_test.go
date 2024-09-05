package deviceplugin_test

import (
	"context"
	"net"
	"os"
	"path"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cns/deviceplugin"
	"github.com/Azure/azure-container-networking/crd/multitenancy/api/v1alpha1"
	"github.com/avast/retry-go/v3"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func TestPluginManagerStartStop(t *testing.T) {
	logger, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("error getting logger: %v", err)
	}

	// start up the fake kubelet
	fakeKubeletSocketDir := os.TempDir()
	kubeletSocket := path.Join(fakeKubeletSocketDir, "kubelet.sock")
	kubeletErrChan := make(chan error)
	vnetPluginRegisterChan := make(chan string)
	ibPluginRegisterChan := make(chan string)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		kubeletErr := runFakeKubelet(ctx, kubeletSocket, vnetPluginRegisterChan, ibPluginRegisterChan, fakeKubeletSocketDir)
		kubeletErrChan <- kubeletErr
	}()

	// run the plugin manager
	expectedVnetNICs := 2
	expectedIBNICs := 3
	manager := deviceplugin.NewPluginManager(logger,
		deviceplugin.PluginManagerSocketPrefix(fakeKubeletSocketDir),
		deviceplugin.PluginManagerKubeletSocket(kubeletSocket),
		deviceplugin.PluginDeviceCheckInterval(time.Second))

	manager.AddPlugin(v1alpha1.DeviceTypeVnetNIC, expectedVnetNICs)
	manager.AddPlugin(v1alpha1.DeviceTypeInfiniBandNIC, expectedIBNICs)

	errChan := make(chan error)
	go func() {
		errChan <- manager.Run(ctx)
	}()

	// wait till the two plugins register themselves with fake kubelet
	vnetPluginEndpoint := <-vnetPluginRegisterChan
	ibPluginEndpoint := <-ibPluginRegisterChan

	// assert the plugin reports the expected vnet nic count
	gotVnetNICCount := getDeviceCount(t, vnetPluginEndpoint)
	if gotVnetNICCount != expectedVnetNICs {
		t.Fatalf("expected %d vnet nics but got %d", expectedVnetNICs, gotVnetNICCount)
	}
	gotIBNICCount := getDeviceCount(t, ibPluginEndpoint)
	if gotIBNICCount != expectedIBNICs {
		t.Fatalf("expected %d ib nics but got %d", expectedIBNICs, gotIBNICCount)
	}

	// update the device counts and assert they match expected after some time
	expectedVnetNICs = 5
	expectedIBNICs = 6
	manager.TrackDevices(v1alpha1.DeviceTypeVnetNIC, expectedVnetNICs)

	manager.TrackDevices(v1alpha1.DeviceTypeInfiniBandNIC, expectedIBNICs)

	checkDeviceCounts := func() error {
		gotVnetNICCount := getDeviceCount(t, vnetPluginEndpoint)
		if gotVnetNICCount != expectedVnetNICs {
			return errors.Errorf("expected %d vnet nics but got %d", expectedVnetNICs, gotVnetNICCount)
		}
		gotIBNICCount := getDeviceCount(t, ibPluginEndpoint)
		if gotIBNICCount != expectedIBNICs {
			return errors.Errorf("expected %d ib nics but got %d", expectedIBNICs, gotIBNICCount)
		}
		return nil
	}

	deviceCountErr := retry.Do(
		checkDeviceCounts,
		retry.Attempts(6),
		retry.Delay(500*time.Millisecond),
	)

	if deviceCountErr != nil {
		t.Fatalf("failed to verify device counts: %v", err)
	}

	// call allocate method and check the response
	req := &v1beta1.AllocateRequest{
		ContainerRequests: []*v1beta1.ContainerAllocateRequest{
			{
				DevicesIDs: []string{"device-0", "device-1"},
			},
		},
	}
	allocateResp := getAllocateResponse(t, vnetPluginEndpoint, req)

	if len(allocateResp.ContainerResponses[0].Envs) != len(req.ContainerRequests[0].DevicesIDs) {
		t.Fatalf("expected allocations %v but received allocations %v", len(req.ContainerRequests[0].DevicesIDs), len(allocateResp.ContainerResponses[0].Envs))
	}

	// call getDevicePluginOptions method
	_, err = getDevicePluginOptionsResponse(vnetPluginEndpoint)
	if err != nil {
		t.Fatalf("error calling getDevicePluginOptions: %v", err)
	}

	// call getPreferredAllocation method
	_, err = getPreferredAllocationResponse(vnetPluginEndpoint)
	if err != nil {
		t.Fatalf("error calling getPreferredAllocation: %v", err)
	}

	// call preStartContainer method
	_, err = getPreStartContainerResponse(vnetPluginEndpoint)
	if err != nil {
		t.Fatalf("error calling PreStartContainer: %v", err)
	}

	// shut down the plugin manager and fake kubelet
	cancel()

	// ensure the plugin manager didn't report an error
	if err := <-errChan; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ensure the fake kubelet didn't report an error
	if err := <-kubeletErrChan; err != nil {
		t.Fatalf("unexpected error from fake kubelet: %v", err)
	}
}

type fakeKubelet struct {
	vnetPluginRegisterChan chan string
	ibPluginRegisterChan   chan string
	pluginPrefix           string
}

func (f *fakeKubelet) Register(_ context.Context, req *v1beta1.RegisterRequest) (*v1beta1.Empty, error) {
	switch req.ResourceName {
	case string(v1alpha1.DeviceTypeVnetNIC):
		f.vnetPluginRegisterChan <- path.Join(f.pluginPrefix, req.Endpoint)
	case string(v1alpha1.DeviceTypeInfiniBandNIC):
		f.ibPluginRegisterChan <- path.Join(f.pluginPrefix, req.Endpoint)
	}
	return &v1beta1.Empty{}, nil
}

func runFakeKubelet(ctx context.Context, address string, vnetPluginRegisterChan, ibPluginRegisterChan chan string, pluginPrefix string) error {
	if err := os.Remove(address); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "error cleaning up previous kubelet socket")
	}

	k := &fakeKubelet{
		vnetPluginRegisterChan: vnetPluginRegisterChan,
		ibPluginRegisterChan:   ibPluginRegisterChan,
		pluginPrefix:           pluginPrefix,
	}
	grpcServer := grpc.NewServer()
	v1beta1.RegisterRegistrationServer(grpcServer, k)

	l, err := net.Listen("unix", address)
	if err != nil {
		return errors.Wrap(err, "error from fake kubelet listening on socket")
	}
	errChan := make(chan error, 2)
	go func() {
		errChan <- grpcServer.Serve(l)
	}()
	defer grpcServer.Stop()

	select {
	case err := <-errChan:
		return errors.Wrap(err, "error running fake kubelet grpc server")
	case <-ctx.Done():
	}
	return nil
}

func getDeviceCount(t *testing.T, pluginAddress string) int {
	conn, err := grpc.Dial(pluginAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), //nolint:staticcheck // TODO: Move to grpc.NewClient method
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			conn, err := d.DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}))
	if err != nil {
		t.Fatalf("error connecting to fake kubelet: %v", err)
	}
	defer conn.Close()

	client := v1beta1.NewDevicePluginClient(conn)
	lwClient, err := client.ListAndWatch(context.Background(), &v1beta1.Empty{})
	if err != nil {
		t.Fatalf("error from listAndWatch: %v", err)
	}

	resp, err := lwClient.Recv()
	if err != nil {
		t.Fatalf("error from listAndWatch Recv: %v", err)
	}

	return len(resp.Devices)
}

func getAllocateResponse(t *testing.T, pluginAddress string, req *v1beta1.AllocateRequest) *v1beta1.AllocateResponse {
	conn, err := grpc.Dial(pluginAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), //nolint:staticcheck // TODO: Move to grpc.NewClient method

		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			conn, err := d.DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}))
	if err != nil {
		t.Fatalf("error connecting to fake kubelet: %v", err)
	}
	defer conn.Close()

	client := v1beta1.NewDevicePluginClient(conn)
	resp, err := client.Allocate(context.Background(), req)
	if err != nil {
		t.Fatalf("error from Allocate: %v", err)
	}
	return resp
}

func getDevicePluginOptionsResponse(pluginAddress string) (*v1beta1.DevicePluginOptions, error) {
	conn, err := grpc.Dial(pluginAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), //nolint:staticcheck // TODO: Move to grpc.NewClient method
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			conn, err := d.DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}))
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to fake kubelet")
	}
	defer conn.Close()

	client := v1beta1.NewDevicePluginClient(conn)
	resp, err := client.GetDevicePluginOptions(context.Background(), &v1beta1.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "error calling GetDevicePluginOptions")
	}
	return resp, nil
}

func getPreferredAllocationResponse(pluginAddress string) (*v1beta1.PreferredAllocationResponse, error) {
	conn, err := grpc.Dial(pluginAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), //nolint:staticcheck // TODO: Move to grpc.NewClient method
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			conn, err := d.DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}))
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to fake kubelet")
	}
	defer conn.Close()

	client := v1beta1.NewDevicePluginClient(conn)
	resp, err := client.GetPreferredAllocation(context.Background(), &v1beta1.PreferredAllocationRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "error calling GetPreferredAllocation")
	}
	return resp, nil
}

func getPreStartContainerResponse(pluginAddress string) (*v1beta1.PreStartContainerResponse, error) {
	conn, err := grpc.Dial(pluginAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), //nolint:staticcheck // TODO: Move to grpc.NewClient method
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := &net.Dialer{}
			conn, err := d.DialContext(ctx, "unix", addr)
			if err != nil {
				return nil, errors.Wrap(err, "failed to dial context")
			}
			return conn, nil
		}))
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to fake kubelet")
	}
	defer conn.Close()

	client := v1beta1.NewDevicePluginClient(conn)
	resp, err := client.PreStartContainer(context.Background(), &v1beta1.PreStartContainerRequest{})
	if err != nil {
		return nil, errors.Wrapf(err, "error calling PreStartContainer")
	}
	return resp, nil
}
