// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"testing"

	"github.com/Azure/azure-container-networking/common"
)

var (

	// Pools and addresses used by tests.
	ipv6subnet1 = "ace:cab:deca:deed::" + testSubnetSize
	ipv6addr2   = "ace:cab:deca:deed::2"
	ipv6addr3   = "ace:cab:deca:deed::3"
)

func createTestIpv6AddressManager() (AddressManager, error) {
	var config common.PluginConfig
	var options map[string]interface{}

	options = make(map[string]interface{})
	options[common.OptEnvironment] = common.OptEnvironmentIPv6NodeIpam

	am, err := NewAddressManager()
	if err != nil {
		return nil, err
	}

	err = am.Initialize(&config, options)
	if err != nil {
		return nil, err
	}

	amImpl := am.(*addressManager)
	src := amImpl.source.(*ipv6IpamSource)
	src.nodeHostname = testNodeName
	src.subnetMaskSizeLimit = testSubnetSize
	src.kubeClient = newKubernetesTestClient()

	return am, nil
}

//
// Address manager tests.
//
// request pool, request address with no address specified, request address with address specified,
// release both addresses, release pool
func TestIPv6GetAddressPoolAndAddress(t *testing.T) {
	// Start with the test address space.
	am, err := createTestIpv6AddressManager()
	if err != nil {
		t.Fatalf("createAddressManager failed, err:%+v.", err)
	}

	// Test if the address spaces are returned correctly.
	local, _ := am.GetDefaultAddressSpaces()

	if local != LocalDefaultAddressSpaceId {
		t.Errorf("GetDefaultAddressSpaces returned invalid local address space.")
	}

	// Request two separate address pools.
	poolID1, subnet1, err := am.RequestPool(LocalDefaultAddressSpaceId, "", "", nil, true)
	if err != nil {
		t.Errorf("RequestPool failed, err:%v", err)
	}

	if subnet1 != ipv6subnet1 {
		t.Errorf("Mismatched retrieved subnet, expected:%+v, actual %+v", ipv6subnet1, subnet1)
	}

	// test with a specified address
	address2, err := am.RequestAddress(LocalDefaultAddressSpaceId, poolID1, ipv6addr2, nil)
	if err != nil {
		t.Errorf("RequestAddress failed, err:%v", err)
	}

	if address2 != ipv6addr2+testSubnetSize {
		t.Errorf("RequestAddress failed, expected: %v, actual: %v", ipv6addr2+testSubnetSize, address2)
	}

	// test with a specified address
	address3, err := am.RequestAddress(LocalDefaultAddressSpaceId, poolID1, "", nil)
	if err != nil {
		t.Errorf("RequestAddress failed, err:%v", err)
	}

	if address3 != ipv6addr3+testSubnetSize {
		t.Errorf("RequestAddress failed, expected: %v, actual: %v", ipv6addr3+testSubnetSize, address3)
	}

	// Release addresses and the pool.
	err = am.ReleaseAddress(LocalDefaultAddressSpaceId, poolID1, address2, nil)
	if err != nil {
		t.Errorf("ReleaseAddress failed, err:%v", err)
	}

	// Release addresses and the pool.
	err = am.ReleaseAddress(LocalDefaultAddressSpaceId, poolID1, address3, nil)
	if err != nil {
		t.Errorf("ReleaseAddress failed, err:%v", err)
	}

	err = am.ReleasePool(LocalDefaultAddressSpaceId, poolID1)
	if err != nil {
		t.Errorf("ReleasePool failed, err:%v", err)
	}
}
