package network

import (
	"net"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/netio"
	"github.com/Azure/azure-container-networking/netlink"
	"github.com/Azure/azure-container-networking/ovsctl"
	"github.com/Azure/azure-container-networking/platform"
)

const (
	bridgeName = "testbridge"
	hostIntf   = "testintf"
)

func TestMain(m *testing.M) {
	// These unit tests run against mock interfaces that do not exist in the test
	// host's netlink tables, so real master resolution cannot succeed. Resolution
	// itself is covered by netio's own unit tests.
	originalResolveMasterInterface := netio.ResolveMasterInterface
	netio.ResolveMasterInterface = func(iface *net.Interface) (*net.Interface, error) {
		return iface, nil
	}
	code := m.Run()
	netio.ResolveMasterInterface = originalResolveMasterInterface
	os.Exit(code)
}

func TestAddRoutes(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")
	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient,
		netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false))

	if err := ovsClient.AddRoutes(nil, ""); err != nil {
		t.Errorf("Add routes failed")
	}
}

func TestDeleteBridge(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient,
		netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false))
	if err := ovsClient.DeleteBridge(); err != nil {
		t.Errorf("Error deleting the OVS bridge: %v", err)
	}
}

func TestAddL2Rules(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")
	extIf := externalInterface{
		Name:       hostIntf,
		MacAddress: []byte("2C:54:91:88:C9:E3"),
	}

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient,
		netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false))
	if err := ovsClient.AddL2Rules(&extIf); err != nil {
		t.Errorf("Unable to add L2 rules: %v", err)
	}
}

func TestDeleteL2Rules(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")
	extIf := externalInterface{
		Name:       hostIntf,
		MacAddress: []byte("2C:54:91:88:C9:E3"),
	}

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient,
		netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false))
	ovsClient.DeleteL2Rules(&extIf)
}

func TestSetBridgeMasterToHostInterface(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient,
		netlink.NewMockNetlink(false, ""), platform.NewMockExecClient(false))
	if err := ovsClient.SetBridgeMasterToHostInterface(); err != nil {
		t.Errorf("Unable to set bridge master to host intf: %v", err)
	}
}
