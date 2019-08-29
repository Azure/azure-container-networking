package ipam

import (
	"net"
	"reflect"
	"runtime"
	"testing"
)

func TestNewMasSource(t *testing.T) {
	options := make(map[string]interface{})
	mas, _ := newMasSource(options)

	if runtime.GOOS == windows {
		if mas.filePath != defaultWindowsFilePath {
			t.Fatalf("default file path set incorrectly")
		}
	} else {
		if mas.filePath != defaultLinuxFilePath {
			t.Fatalf("default file path set incorrectly")
		}
	}
	if mas.name != "MAS" {
		t.Fatalf("mas source Name incorrect")
	}
}

func TestGetSDNInterfaces(t *testing.T) {
	const validFileName = "testfiles/masInterfaceConfig.json"
	const invalidFileName = "mas_test.go"
	const nonexistentFileName = "bad"

	interfaces, err := getSDNInterfaces(validFileName)
	if err != nil {
		t.Fatalf("failed to get sdn Interfaces from file: %v", err)
	}

	correctInterfaces := &NetworkInterfaces{
		Interfaces: []Interface{
			{
				MacAddress: "000D3A6E1825",
				IsPrimary:  true,
				IPSubnets: []IPSubnet{
					{
						Prefix: "1.0.0.0/12",
						IPAddresses: []IPAddress{
							{Address: "1.0.0.4", IsPrimary: true},
							{Address: "1.0.0.5", IsPrimary: false},
							{Address: "1.0.0.6", IsPrimary: false},
							{Address: "1.0.0.7", IsPrimary: false},
						},
					},
				},
			},
		},
	}

	if !reflect.DeepEqual(interfaces, correctInterfaces) {
		t.Fatalf("Interface list did not match expected list. expected: %v, actual: %v", interfaces, correctInterfaces)
	}

	interfaces, err = getSDNInterfaces(invalidFileName)
	if interfaces != nil || err == nil {
		t.Fatal("didn't throw error on invalid file")
	}

	interfaces, err = getSDNInterfaces(nonexistentFileName)
	if interfaces != nil || err == nil {
		t.Fatal("didn't throw error on nonexistent file")
	}
}

func TestPopulateAddressSpace(t *testing.T) {
	hardwareAddress0, _ := net.ParseMAC("00:00:00:00:00:00")
	hardwareAddress1, _ := net.ParseMAC("11:11:11:11:11:11")
	hardwareAddress2, _ := net.ParseMAC("00:0d:3a:6e:18:25")

	localInterfaces := []net.Interface{
		{HardwareAddr: hardwareAddress0, Name: "eth0"},
		{HardwareAddr: hardwareAddress1, Name: "eth1"},
		{HardwareAddr: hardwareAddress2, Name: "eth2"},
	}

	local := &addressSpace{
		Id:    LocalDefaultAddressSpaceId,
		Scope: LocalScope,
		Pools: make(map[string]*addressPool),
	}

	sdnInterfaces := &NetworkInterfaces{
		Interfaces: []Interface{
			{
				MacAddress: "000D3A6E1825",
				IsPrimary:  true,
				IPSubnets: []IPSubnet{
					{
						Prefix: "1.0.0.0/12",
						IPAddresses: []IPAddress{
							{Address: "1.1.1.5", IsPrimary: true},
							{Address: "1.1.1.6", IsPrimary: false},
							{Address: "1.1.1.6", IsPrimary: false},
							{Address: "1.1.1.7", IsPrimary: false},
							{Address: "invalid", IsPrimary: false},
						},
					},
				},
			},
		},
	}

	err := populateAddressSpace(local, sdnInterfaces, localInterfaces)
	if err != nil {
		t.Fatalf("Error populating address space: %v", err)
	}

	if len(local.Pools) != 1 {
		t.Fatalf("Pool list has incorrect length. expected: %d, actual: %d", 1, len(local.Pools))
	}

	pool, ok := local.Pools["1.0.0.0/12"]
	if !ok {
		t.Fatal("Address pool 1.0.0.0/12 missing")
	}

	if pool.IfName != "eth2" {
		t.Fatalf("Incorrect interface name. expected: %s, actual %s", "eth2", pool.IfName)
	}

	if pool.Priority != 0 {
		t.Fatalf("Incorrect interface priority. expected: %d, actual %d", 0, pool.Priority)
	}

	if len(pool.Addresses) != 2 {
		t.Fatalf("Address list has incorrect length. expected: %d, actual: %d", 2, len(pool.Addresses))
	}

	_, ok = pool.Addresses["1.1.1.6"]
	if !ok {
		t.Fatal("Address 1.1.1.6 missing")
	}

	_, ok = pool.Addresses["1.1.1.7"]
	if !ok {
		t.Fatal("Address 1.1.1.7 missing")
	}
}

func TestPopulateAddressSpaceMultipleSDNInterfaces(t *testing.T) {
	hardwareAddress0, _ := net.ParseMAC("00:00:00:00:00:00")
	hardwareAddress1, _ := net.ParseMAC("11:11:11:11:11:11")
	localInterfaces := []net.Interface{
		{HardwareAddr: hardwareAddress0, Name: "eth0"},
		{HardwareAddr: hardwareAddress1, Name: "eth1"},
	}

	local := &addressSpace{
		Id:    LocalDefaultAddressSpaceId,
		Scope: LocalScope,
		Pools: make(map[string]*addressPool),
	}

	sdnInterfaces := &NetworkInterfaces{
		Interfaces: []Interface{
			{
				MacAddress: "000000000000",
				IsPrimary:  true,
				IPSubnets: []IPSubnet{
					{
						Prefix: "0.0.0.0/24",
						IPAddresses: []IPAddress{},
					},
					{
						Prefix: "0.1.0.0/24",
						IPAddresses: []IPAddress{},
					},
					{
						Prefix: "0.0.0.0/24",
					},
					{
						Prefix: "invalid",
					},
				},
			},
			{
				MacAddress: "111111111111",
				IsPrimary: false,
				IPSubnets: []IPSubnet{
					{
						Prefix: "1.0.0.0/24",
						IPAddresses: []IPAddress{},
					},
					{
						Prefix: "1.1.0.0/24",
						IPAddresses: []IPAddress{},
					},
				},
			},
			{
				MacAddress: "222222222222",
				IsPrimary: false,
				IPSubnets: []IPSubnet{},
			},
		},
	}

	err := populateAddressSpace(local, sdnInterfaces, localInterfaces)
	if err != nil {
		t.Fatalf("Error populating address space: %v", err)
	}

	if len(local.Pools) != 4 {
		t.Fatalf("Pool list has incorrect length. expected: %d, actual: %d", 4, len(local.Pools))
	}

	pool, ok := local.Pools["0.0.0.0/24"]
	if !ok {
		t.Fatal("Address pool 0.0.0.0/24 missing")
	}

	if pool.IfName != "eth0" {
		t.Fatalf("Incorrect interface name. expected: %s, actual %s", "eth0", pool.IfName)
	}

	if pool.Priority != 0 {
		t.Fatalf("Incorrect interface priority. expected: %d, actual %d", 0, pool.Priority)
	}

	pool, ok = local.Pools["0.1.0.0/24"]
	if !ok {
		t.Fatal("Address pool 0.1.0.0/24 missing")
	}

	if pool.IfName != "eth0" {
		t.Fatalf("Incorrect interface name. expected: %s, actual %s", "eth0", pool.IfName)
	}

	if pool.Priority != 0 {
		t.Fatalf("Incorrect interface priority. expected: %d, actual %d", 0, pool.Priority)
	}

	pool, ok = local.Pools["1.0.0.0/24"]
	if !ok {
		t.Fatal("Address pool 1.0.0.0/24 missing")
	}

	if pool.IfName != "eth1" {
		t.Fatalf("Incorrect interface name. expected: %s, actual %s", "eth1", pool.IfName)
	}

	if pool.Priority != 1 {
		t.Fatalf("Incorrect interface priority. expected: %d, actual %d", 1, pool.Priority)
	}

	pool, ok = local.Pools["1.1.0.0/24"]
	if !ok {
		t.Fatal("Address pool 1.1.0.0/24 missing")
	}

	if pool.IfName != "eth1" {
		t.Fatalf("Incorrect interface name. expected: %s, actual %s", "eth1", pool.IfName)
	}

	if pool.Priority != 1 {
		t.Fatalf("Incorrect interface priority. expected: %d, actual %d", 1, pool.Priority)
	}
}