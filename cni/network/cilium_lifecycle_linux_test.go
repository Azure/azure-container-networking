package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/cni"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/ipam/ciliumpool"
	"github.com/Azure/azure-container-networking/iptables"
	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	acnnetwork "github.com/Azure/azure-container-networking/network"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

const (
	lifecycleIP      = "10.0.0.10"
	lifecycleName    = "azure-lifecycle"
	lifecycleMode    = "transparent-tunnel"
	lifecycleOld     = "old-sandbox"
	lifecycleNew     = "new-sandbox"
	lifecycleOldVeth = "old-veth"
	lifecycleNewVeth = "new-veth"
)

var (
	errLifecycleWrite = errors.New("injected endpoint state-write failure")
	errLifecycleCall  = errors.New("unexpected lifecycle OS call")
)

func TestCiliumStaleDELAfterStateWriteFailure(t *testing.T) {
	allocator, err := lifecycleAllocator()
	if err != nil {
		t.Fatal(err)
	}
	oldOwner := ciliumpool.Owner{Network: lifecycleName, Sandbox: lifecycleOld, Interface: eth0IfName}
	ip, err := allocator.Add(t.Context(), oldOwner)
	if err != nil || ip != lifecycleIP {
		t.Fatalf("initial allocation = %q, %v", ip, err)
	}
	oldEndpoint := lifecycleEndpoint(lifecycleOld, lifecycleOldVeth)
	snapshot, err := lifecycleSnapshot(oldEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	state := &lifecycleStore{data: snapshot, fail: true}
	dataplane := &lifecycleDataplane{
		members: map[string]bool{ip: true},
		marks:   map[string]bool{"-i " + lifecycleOldVeth: true},
		routes:  map[int]bool{1: true},
	}
	plugin, err := lifecyclePlugin(state, dataplane, allocator, oldOwner)
	if err != nil {
		t.Fatal(err)
	}
	cfg := cni.NetworkConfig{Name: lifecycleName, Mode: lifecycleMode, IPAM: cni.IPAM{Type: cni.AzureCiliumIPAM}}
	args := &skel.CmdArgs{ContainerID: lifecycleOld, IfName: eth0IfName, StdinData: cfg.Serialize()}
	if deleteErr := plugin.Delete(args); deleteErr == nil || !strings.Contains(deleteErr.Error(), errLifecycleWrite.Error()) {
		t.Fatalf("DEL did not reach the injected state-write failure: %v", deleteErr)
	}
	if state.fail || !cmp.Equal(snapshot, state.data) || len(dataplane.members)+len(dataplane.marks)+len(dataplane.routes) != 0 {
		t.Fatal("first DEL did not remove networking while retaining the failed disk snapshot")
	}
	newOwner := oldOwner
	newOwner.Sandbox = lifecycleNew
	reusedIP, err := allocator.Add(t.Context(), newOwner)
	if err != nil || reusedIP != ip {
		t.Fatalf("released IP was not reused: %q, %v", reusedIP, err)
	}

	// A replacement ADD persists its endpoint alongside the stale disk record.
	state.data, err = lifecycleSnapshot(oldEndpoint, lifecycleEndpoint(lifecycleNew, lifecycleNewVeth))
	if err != nil {
		t.Fatal(err)
	}
	dataplane.members[ip], dataplane.marks["-i "+lifecycleNewVeth], dataplane.routes[2] = true, true, true
	plugin, err = lifecyclePlugin(state, dataplane, allocator, oldOwner)
	if err != nil {
		t.Fatal(err)
	}
	if deleteErr := plugin.Delete(args); deleteErr != nil {
		t.Fatal(deleteErr)
	}
	if checkErr := allocator.Check(t.Context(), newOwner); checkErr != nil {
		t.Fatalf("stale DEL removed replacement reservation: %v", checkErr)
	}
	if !dataplane.members[ip] || !dataplane.marks["-i "+lifecycleNewVeth] || !dataplane.routes[2] {
		t.Fatalf("stale DEL damaged replacement dataplane: members=%v marks=%v routes=%v",
			dataplane.members, dataplane.marks, dataplane.routes)
	}
	reloaded, err := lifecyclePlugin(state, dataplane, allocator, oldOwner)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.nm.GetEndpointInfosFromContainerID(lifecycleOld)) != 0 ||
		len(reloaded.nm.GetEndpointInfosFromContainerID(lifecycleNew)) != 1 {
		t.Fatal("retry did not persist deletion of only the stale endpoint")
	}
}

type lifecycleStore struct {
	store.KeyValueStore
	data []byte
	fail bool
}

func (s *lifecycleStore) Read(_ string, value any) error {
	if err := json.Unmarshal(s.data, value); err != nil {
		return fmt.Errorf("reading endpoint snapshot: %w", err)
	}
	return nil
}

func (s *lifecycleStore) Write(_ string, value any) error {
	if s.fail {
		s.fail = false
		return errLifecycleWrite
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("writing endpoint snapshot: %w", err)
	}
	s.data = data
	return nil
}

type lifecycleDataplane struct {
	netlink.NetlinkInterface
	netio.NetIOInterface
	platform.ExecClient
	*iptables.Client
	members map[string]bool
	marks   map[string]bool
	routes  map[int]bool
}

func (d *lifecycleDataplane) ExecuteCommand(_ context.Context, command string, args ...string) (string, error) {
	if command != "ipset" || len(args) != 4 || args[0] != "del" || args[1] != "azure-tt-local-pods" {
		return "", errLifecycleCall
	}
	delete(d.members, args[2])
	return "", nil
}

func (d *lifecycleDataplane) GetNetworkInterfaceByName(name string) (*net.Interface, error) {
	switch name {
	case lifecycleOldVeth:
		return &net.Interface{Name: name, Index: 1}, nil
	case lifecycleNewVeth:
		return &net.Interface{Name: name, Index: 2}, nil
	default:
		return nil, errLifecycleCall
	}
}

func (d *lifecycleDataplane) DeleteIPRoute(route *netlink.Route) error {
	delete(d.routes, route.LinkIndex)
	return nil
}

func (d *lifecycleDataplane) DeleteIptableRule(_, table, chain, match, target string) error {
	if table != iptables.Mangle || chain != iptables.Prerouting || target != "MARK --set-mark 3" {
		return errLifecycleCall
	}
	delete(d.marks, match)
	return nil
}

type lifecycleDelegate struct {
	mockDelegatePlugin
	allocator *ciliumpool.Allocator
	owner     ciliumpool.Owner
}

func (d *lifecycleDelegate) DelegateDel(_ string, cfg *cni.NetworkConfig) error {
	if err := d.allocator.Delete(context.Background(), d.owner, cfg.IPAM.Address); err != nil {
		return fmt.Errorf("releasing lifecycle reservation: %w", err)
	}
	return nil
}

func lifecyclePlugin(state *lifecycleStore, dataplane *lifecycleDataplane, allocator *ciliumpool.Allocator, owner ciliumpool.Owner) (*NetPlugin, error) {
	manager, err := acnnetwork.NewNetworkManager(dataplane, dataplane, dataplane, nil, dataplane, nil)
	if err != nil {
		return nil, fmt.Errorf("creating lifecycle manager: %w", err)
	}
	if err := manager.Initialize(&common.PluginConfig{Store: state}, false); err != nil {
		return nil, fmt.Errorf("restoring lifecycle manager: %w", err)
	}
	plugin := GetTestResources()
	plugin.nm = manager
	plugin.ipamInvoker = &AzureIPAMInvoker{
		plugin: &lifecycleDelegate{allocator: allocator, owner: owner}, nwInfo: &acnnetwork.EndpointInfo{},
	}
	return plugin, nil
}

func lifecycleEndpoint(sandbox, veth string) map[string]any {
	return map[string]any{
		"Id": sandbox, "ContainerID": sandbox, "IfName": eth0IfName, "HostIfName": veth,
		"IPAddresses": []net.IPNet{*getCIDRNotationForAddress(lifecycleIP + "/24")},
	}
}

func lifecycleSnapshot(endpoints ...map[string]any) ([]byte, error) {
	records := map[string]any{}
	for _, endpoint := range endpoints {
		records[endpoint["Id"].(string)] = endpoint
	}
	data, err := json.Marshal(map[string]any{"ExternalInterfaces": map[string]any{eth0IfName: map[string]any{
		"Name": eth0IfName, "Networks": map[string]any{lifecycleName: map[string]any{
			"Id": lifecycleName, "Mode": lifecycleMode, "Endpoints": records,
		}},
	}}})
	if err != nil {
		return nil, fmt.Errorf("creating endpoint snapshot: %w", err)
	}
	return data, nil
}

func lifecycleAllocator() (*ciliumpool.Allocator, error) {
	const nodeJSON = `{
		"apiVersion":"cilium.io/v2","kind":"CiliumNode",
		"metadata":{"name":"node","uid":"cilium-node","resourceVersion":"1",
			"annotations":{"azure-container-networking.io/vm-id":"vm","azure-container-networking.io/ipam-consumer":"azure-cilium-ipam"},
			"ownerReferences":[{"apiVersion":"v1","kind":"Node","name":"node","uid":"node-uid"}]},
		"spec":{"instance-id":"instance","ipam":{"pre-allocate":1,"pool":{"10.0.0.10":{"resource":"nic"}}}},
		"status":{"azure":{"interfaces":[{"id":"nic","ip":"10.0.0.4","mac":"00:11:22:33:44:55",
			"gateway":"10.0.0.1","subnet":{"cidr":"10.0.0.0/24"},
			"addresses":[{"ip":"10.0.0.10","state":"succeeded"}]}]}}}`
	node := &unstructured.Unstructured{}
	if err := node.UnmarshalJSON([]byte(nodeJSON)); err != nil {
		return nil, fmt.Errorf("decoding CiliumNode fixture: %w", err)
	}
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), node)
	return &ciliumpool.Allocator{
		Nodes: client.Resource(ciliumpool.CiliumNodes), NodeName: node.GetName(),
		BootID: "11111111-1111-4111-8111-111111111111", PreAllocate: 1, Supply: map[string]bool{lifecycleIP: true},
		Binding: ciliumpool.Binding{
			NodeName: node.GetName(), NodeUID: string(node.GetOwnerReferences()[0].UID),
			VMID: "vm", Resource: "nic", Subnet: v4NetCidr,
			InstanceID: "instance", Primary: "10.0.0.4", MAC: "00:11:22:33:44:55",
		},
	}, nil
}
