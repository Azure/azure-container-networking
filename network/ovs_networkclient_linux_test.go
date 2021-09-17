package network

import (
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/ovsctl"
)

const (
	bridgeName = "testbridge"
	hostIntf   = "testintf"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestAddRoutes(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")
	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient)

	if err := ovsClient.AddRoutes(nil, ""); err != nil {
		t.Errorf("Add routes failed")
	}
}

func TestCreateBridge(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")
	f, err := os.Create(ovsConfigFile)
	if err != nil {
		t.Errorf("Unable to create %v before test: %v", ovsConfigFile, err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString("FORCE_COREFILES=yes"); err != nil {
		t.Errorf("Unable to write to file %v: %v", ovsConfigFile, err)
	}

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient)
	if err := ovsClient.CreateBridge(); err != nil {
		t.Errorf("Error creating OVS bridge: %v", err)
	}

	os.Remove(ovsConfigFile)
}

func TestDeleteBridge(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient)
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

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient)
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

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient)
	ovsClient.DeleteL2Rules(&extIf)
}

func TestSetBridgeMasterToHostInterface(t *testing.T) {
	ovsctlClient := ovsctl.NewMockOvsctl(false, "", "")

	ovsClient := NewOVSClient(bridgeName, hostIntf, ovsctlClient)
	if err := ovsClient.SetBridgeMasterToHostInterface(); err != nil {
		t.Errorf("Unable to set bridge master to host intf: %v", err)
	}
}
