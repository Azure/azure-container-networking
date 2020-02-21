// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"errors"
	"fmt"
	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/testutils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net"
	"testing"
	"time"
)

var (
	anyInterface = "any"
	anyPriority  = 42

	// Pools and addresses used by tests.
	subnet1 = net.IPNet{IP: net.IPv4(10, 0, 1, 0), Mask: net.IPv4Mask(255, 255, 255, 0)}
	addr11  = net.IPv4(10, 0, 1, 1)
	addr12  = net.IPv4(10, 0, 1, 2)
	addr13  = net.IPv4(10, 0, 1, 3)

	subnet2 = net.IPNet{IP: net.IPv4(10, 0, 2, 0), Mask: net.IPv4Mask(255, 255, 255, 0)}
	addr21  = net.IPv4(10, 0, 2, 1)
	addr22  = net.IPv4(10, 0, 2, 2)
	addr23  = net.IPv4(10, 0, 2, 3)

	subnet3 = net.IPNet{IP: net.IPv4(10, 0, 3, 0), Mask: net.IPv4Mask(255, 255, 255, 0)}
	addr31  = net.IPv4(10, 0, 3, 1)
	addr32  = net.IPv4(10, 0, 3, 2)
	addr33  = net.IPv4(10, 0, 3, 3)
)

// createAddressManager creates an address manager with a simple test configuration.
func createAddressManager(options map[string]interface{}) (AddressManager, error) {
	var config common.PluginConfig

	am, err := NewAddressManager()
	if err != nil {
		return nil, err
	}

	err = am.Initialize(&config, options)
	if err != nil {
		return nil, err
	}

	err = setupTestAddressSpace(am)
	if err != nil {
		return nil, err
	}

	return am, nil
}

// dumpAddressManager dumps the contents of an address manager.
func dumpAddressManager(am AddressManager) {
	amImpl := am.(*addressManager)
	fmt.Printf("AddressManager:%+v\n", amImpl)
	for sk, sv := range amImpl.AddrSpaces {
		fmt.Printf("AddressSpace %v:%+v\n", sk, sv)
		for pk, pv := range sv.Pools {
			fmt.Printf("\tPool %v:%+v\n", pk, pv)
			for ak, av := range pv.Addresses {
				fmt.Printf("\t\tAddress %v:%+v\n", ak, av)
			}
		}
	}
}

// setupTestAddressSpace creates a simple address space used by various tests.
func setupTestAddressSpace(am AddressManager) error {
	var anyInterface = "any"
	var anyPriority = 42

	amImpl := am.(*addressManager)

	// Configure an empty global address space.
	globalAs, err := amImpl.newAddressSpace(GlobalDefaultAddressSpaceId, GlobalScope)
	if err != nil {
		return err
	}

	err = amImpl.setAddressSpace(globalAs)
	if err != nil {
		return err
	}

	// Configure a local address space.
	localAs, err := amImpl.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	if err != nil {
		return err
	}

	// Add subnet1 with addresses addr11 and addr12.
	ap, err := localAs.newAddressPool(anyInterface, anyPriority, &subnet1)
	ap.newAddressRecord(&addr11)
	ap.newAddressRecord(&addr12)

	// Add subnet2 with addr21.
	ap, err = localAs.newAddressPool(anyInterface, anyPriority, &subnet2)
	ap.newAddressRecord(&addr21)

	amImpl.setAddressSpace(localAs)

	return nil
}

// cleanupTestAddressSpace deletes any existing address spaces.
func cleanupTestAddressSpace(am AddressManager) error {
	amImpl := am.(*addressManager)

	// Configure an empty local address space.
	localAs, err := amImpl.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
	if err != nil {
		return err
	}

	err = amImpl.setAddressSpace(localAs)
	if err != nil {
		return err
	}

	// Configure an empty global address space.
	globalAs, err := amImpl.newAddressSpace(GlobalDefaultAddressSpaceId, GlobalScope)
	if err != nil {
		return err
	}

	err = amImpl.setAddressSpace(globalAs)
	if err != nil {
		return err
	}

	return nil
}

//
// Address manager tests.
//

func TestManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manager Suite")
}

var (
	_ = Describe("Test Manager", func() {

		Describe("Test Initialize", func() {
			Context("When store is nil", func() {
				It("Initialize return nil", func() {
					var config common.PluginConfig
					config.Store = nil
					options := map[string]interface{}{}
					options[common.OptEnvironment] = ""
					am, err := NewAddressManager()
					Expect(am).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					err = am.Initialize(&config, options)
					Expect(err).To(BeNil())
				})
			})

			Context("When restore key not found", func() {
				It("Initialize return nil", func() {
					var config common.PluginConfig
					storeMock := &testutils.KeyValueStoreMock{}
					storeMock.ReadError = store.ErrKeyNotFound
					config.Store = storeMock
					options := map[string]interface{}{}
					options[common.OptEnvironment] = ""
					am, err := NewAddressManager()
					Expect(am).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					err = am.Initialize(&config, options)
					Expect(err).To(BeNil())
				})
			})

			Context("When restore return error", func() {
				It("Initialize return error", func() {
					var config common.PluginConfig
					storeMock := &testutils.KeyValueStoreMock{}
					storeMock.ReadError = errors.New("Error")
					config.Store = storeMock
					options := map[string]interface{}{}
					options[common.OptEnvironment] = ""
					am, err := NewAddressManager()
					Expect(am).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					err = am.Initialize(&config, options)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("When StartSource fail", func() {
				It("Initialize return error", func() {
					var config common.PluginConfig
					options := map[string]interface{}{}
					options[common.OptEnvironment] = "Invalid"
					am, err := NewAddressManager()
					Expect(am).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					err = am.Initialize(&config, options)
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Describe("Test restore", func() {
			Context("When store is nil", func() {
				It("restore return nil", func() {
					am := &addressManager{
						AddrSpaces: make(map[string]*addressSpace),
					}
					err := am.restore()
					Expect(err).To(BeNil())
				})
			})

			Context("Test Populate pointers", func() {
				It("Should build addrsByID successfully", func() {
					am := &addressManager{
						AddrSpaces: make(map[string]*addressSpace),
					}
					timeReboot, _ := platform.GetLastRebootTime()
					am.store = &testutils.KeyValueStoreMock{
						ModificationTime: timeReboot.Add(time.Hour),
					}
					ap := &addressPool{
						Id:			"ap-test",
						RefCount: 	1,
						Addresses: 	make(map[string]*addressRecord),
					}
					ap.Addresses["ar-test"] = &addressRecord{
						ID:		"ar-test",
						InUse: 	true,
					}
					as := &addressSpace{
						Id:		"as-test",
						Pools: 	make(map[string]*addressPool),
					}
					as.Pools["ap-test"] = ap
					am.AddrSpaces["as-test"] = as
					err := am.restore()
					Expect(err).To(BeNil())
					as = am.AddrSpaces["as-test"]
					ap = as.Pools["ap-test"]
					ar := ap.addrsByID["ar-test"]
					Expect(ar.ID).To(Equal("ar-test"))
					Expect(ap.RefCount).To(Equal(1))
					Expect(ar.InUse).To(BeTrue())
				})
			})

			Context("When GetModificationTime return error", func() {
				It("Should not clear the RefCount and InUse", func() {
					am := &addressManager{
						AddrSpaces: make(map[string]*addressSpace),
					}
					am.store = &testutils.KeyValueStoreMock{
						GetModificationTimeError: errors.New("Error"),
					}
					ap := &addressPool{
						Id:			"ap-test",
						RefCount: 	1,
						Addresses: 	make(map[string]*addressRecord),
					}
					ap.Addresses["ar-test"] = &addressRecord{
						ID:		"ar-test",
						InUse: 	true,
					}
					as := &addressSpace{
						Id:		"as-test",
						Pools: 	make(map[string]*addressPool),
					}
					as.Pools["ap-test"] = ap
					am.AddrSpaces["as-test"] = as
					err := am.restore()
					Expect(err).To(BeNil())
					as = am.AddrSpaces["as-test"]
					ap = as.Pools["ap-test"]
					ar := ap.addrsByID["ar-test"]
					Expect(ar.ID).To(Equal("ar-test"))
					Expect(ap.RefCount).To(Equal(1))
					Expect(ar.InUse).To(BeTrue())
				})
			})

			Context("When rebooted", func() {
				It("Should clear the RefCount and InUse", func() {
					am := &addressManager{
						AddrSpaces: make(map[string]*addressSpace),
					}
					am.store = &testutils.KeyValueStoreMock{}
					ap := &addressPool{
						Id:			"ap-test",
						RefCount: 	1,
						Addresses: 	make(map[string]*addressRecord),
					}
					ap.Addresses["ar-test"] = &addressRecord{
						ID:		"ar-test",
						InUse: 	true,
					}
					as := &addressSpace{
						Id:		"as-test",
						Pools: 	make(map[string]*addressPool),
					}
					as.Pools["ap-test"] = ap
					am.AddrSpaces["as-test"] = as
					err := am.restore()
					Expect(err).To(BeNil())
					as = am.AddrSpaces["as-test"]
					ap = as.Pools["ap-test"]
					ar := ap.addrsByID["ar-test"]
					Expect(ar.ID).To(Equal("ar-test"))
					Expect(ap.RefCount).To(Equal(0))
					Expect(ar.InUse).To(BeFalse())
				})
			})
		})

		Describe("Test save", func() {
			Context("When store is nill", func() {
				It("Should return nil", func() {
					am := &addressManager{}
					err := am.save()
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Describe("Test StartSource", func() {
			Context("When environment is azure", func() {
				It("Should return azure source", func() {
					am := &addressManager{}
					options := map[string]interface{}{}
					options[common.OptEnvironment] = common.OptEnvironmentAzure
					err := am.StartSource(options)
					Expect(err).NotTo(HaveOccurred())
					Expect(am.source).NotTo(BeNil())
				})
			})

			Context("When environment is mas", func() {
				It("Should return mas", func() {
					am := &addressManager{}
					options := map[string]interface{}{}
					options[common.OptEnvironment] = common.OptEnvironmentMAS
					err := am.StartSource(options)
					Expect(err).NotTo(HaveOccurred())
					Expect(am.source).NotTo(BeNil())
				})
			})

			Context("When environment is null", func() {
				It("Should return null source", func() {
					am := &addressManager{}
					options := map[string]interface{}{}
					options[common.OptEnvironment] = "null"
					err := am.StartSource(options)
					Expect(err).NotTo(HaveOccurred())
					Expect(am.source).NotTo(BeNil())
				})
			})

			Context("When environment is nil", func() {
				It("Should return nil", func() {
					am := &addressManager{}
					options := map[string]interface{}{}
					options[common.OptEnvironment] = ""
					err := am.StartSource(options)
					Expect(err).NotTo(HaveOccurred())
					Expect(am.source).To(BeNil())
				})
			})

			Context("When environment is nil", func() {
				It("Should return nil", func() {
					am := &addressManager{}
					options := map[string]interface{}{}
					options[common.OptEnvironment] = "Invalid"
					err := am.StartSource(options)
					Expect(err).To(HaveOccurred())
					Expect(am.source).To(BeNil())
				})
			})
		})

		Describe("Test GetDefaultAddressSpaces", func() {
			Context("When local and global are nil", func() {
				It("Should return empty string", func() {
					am := &addressManager{
						AddrSpaces: make(map[string]*addressSpace),
					}
					localId, globalId := am.GetDefaultAddressSpaces()
					Expect(localId).To(BeEmpty())
					Expect(globalId).To(BeEmpty())
				})
			})

			Context("When local and global are nil", func() {
				It("Should return empty string", func() {
					am := &addressManager{
						AddrSpaces: make(map[string]*addressSpace),
					}
					am.AddrSpaces[LocalDefaultAddressSpaceId] = &addressSpace{Id:"localId"}
					am.AddrSpaces[GlobalDefaultAddressSpaceId] = &addressSpace{Id:"globalId"}
					localId, globalId := am.GetDefaultAddressSpaces()
					Expect(localId).To(Equal("localId"))
					Expect(globalId).To(Equal("globalId"))
				})
			})
		})

		// Tests updating an existing address space adds new resources and removes stale ones.
		Describe("Test Address Space Update", func() {
			Context("When update the Address Space", func() {
				It("Should update successfully", func() {
					// Start with the test address space.
					options := map[string]interface{}{}
					am, err := createAddressManager(options)
					Expect(err).ToNot(HaveOccurred())
		
					amImpl := am.(*addressManager)
		
					// Create a new local address space to update the existing one.
					localAs, err := amImpl.newAddressSpace(LocalDefaultAddressSpaceId, LocalScope)
					Expect(err).ToNot(HaveOccurred())
		
					// Remove addr12 and add addr13 in subnet1.
					ap, err := localAs.newAddressPool(anyInterface, anyPriority, &subnet1)
					Expect(err).ToNot(HaveOccurred())
					_, err = ap.newAddressRecord(&addr11)
					Expect(err).ToNot(HaveOccurred())
					_, err = ap.newAddressRecord(&addr13)
					Expect(err).ToNot(HaveOccurred())
		
					// Remove subnet2.
					// Add subnet3 with addr31.
					ap, err = localAs.newAddressPool(anyInterface, anyPriority, &subnet3)
					Expect(err).ToNot(HaveOccurred())
					_, err = ap.newAddressRecord(&addr31)
					Expect(err).ToNot(HaveOccurred())
		
					err = amImpl.setAddressSpace(localAs)
					Expect(err).ToNot(HaveOccurred())
		
					// Test that the address space was updated correctly.
					localAs, err = amImpl.getAddressSpace(LocalDefaultAddressSpaceId)
					Expect(err).ToNot(HaveOccurred())
		
					// Subnet1 should have addr11 and addr13, but not addr12.
					ap, err = localAs.getAddressPool(subnet1.String())
					Expect(err).ToNot(HaveOccurred())
		
					_, err = ap.requestAddress(addr11.String(), nil)
					Expect(err).ToNot(HaveOccurred())
		
					_, err = ap.requestAddress(addr12.String(), nil)
					Expect(err).To(HaveOccurred())
		
					_, err = ap.requestAddress(addr13.String(), nil)
					Expect(err).ToNot(HaveOccurred())
		
					// Subnet2 should not exist.
					ap, err = localAs.getAddressPool(subnet2.String())
					Expect(err).To(HaveOccurred())
		
					// Subnet3 should have addr31 only.
					ap, err = localAs.getAddressPool(subnet3.String())
					Expect(err).ToNot(HaveOccurred())
		
					_, err = ap.requestAddress(addr31.String(), nil)
					Expect(err).ToNot(HaveOccurred())
		
					_, err = ap.requestAddress(addr32.String(), nil)
					Expect(err).To(HaveOccurred())
				})
			})
		})
		
		Describe("Test Address Pool requests", func() {
			// Tests multiple wildcard address pool requests return separate pools.
			Context("When request pool for separate pools", func() {
				It("Should request and release pool successfully", func() {
					// Start with the test address space.
					options := map[string]interface{}{}
					am, err := createAddressManager(options)
					Expect(err).ToNot(HaveOccurred())
		
					// Request two separate address pools.
					poolId1, subnet1, err := am.RequestPool(LocalDefaultAddressSpaceId, "", "", nil, false)
					Expect(err).ToNot(HaveOccurred())
		
					poolId2, subnet2, err := am.RequestPool(LocalDefaultAddressSpaceId, "", "", nil, false)
					Expect(err).ToNot(HaveOccurred())
		
					// Test the poolIds and subnets do not match.
					Expect(poolId1).ToNot(Equal(poolId2))
					Expect(subnet1).ToNot(Equal(subnet2))
		
					// Release the address pools.
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolId1)
					Expect(err).ToNot(HaveOccurred())
		
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolId2)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		
			// Tests multiple identical address pool requests return the same pool and pools are referenced correctly.
			Context("When request pool for same pool", func() {
				It("Should request and release pool successfully", func() {
					// Start with the test address space.
					options := map[string]interface{}{}
					am, err := createAddressManager(options)
					Expect(err).ToNot(HaveOccurred())
		
					// Request the same address pool twice.
					poolId1, subnet1, err := am.RequestPool(LocalDefaultAddressSpaceId, "", "", nil, false)
					Expect(err).ToNot(HaveOccurred())
		
					poolId2, subnet2, err := am.RequestPool(LocalDefaultAddressSpaceId, poolId1, "", nil, false)
					Expect(err).ToNot(HaveOccurred())
		
					// Test the subnets do not match.
					Expect(poolId1).To(Equal(poolId2))
					Expect(subnet1).To(Equal(subnet2))
		
					// Release the address pools.
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolId1)
					Expect(err).ToNot(HaveOccurred())
		
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolId2)
					Expect(err).ToNot(HaveOccurred())
		
					// Third release should fail.
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolId1)
					Expect(err).To(HaveOccurred())
				})
			})
		})
		
		Describe("Test Address requests", func() {
			// Tests address requests from the same pool return separate addresses and releases work correctly.
			Context("When request from the same pool", func() {
				It("Should request and release successfully", func() {
					// Start with the test address space.
					options := map[string]interface{}{}
					am, err := createAddressManager(options)
					Expect(err).ToNot(HaveOccurred())
		
					// Request a pool.
					poolId, _, err := am.RequestPool(LocalDefaultAddressSpaceId, "", "", nil, false)
					Expect(err).ToNot(HaveOccurred())
		
					// Request two addresses from the pool.
					address1, err := am.RequestAddress(LocalDefaultAddressSpaceId, poolId, "", nil)
					Expect(err).ToNot(HaveOccurred())
		
					addr, _, _ := net.ParseCIDR(address1)
					address1 = addr.String()
		
					address2, err := am.RequestAddress(LocalDefaultAddressSpaceId, poolId, "", nil)
					Expect(err).ToNot(HaveOccurred())
		
					addr, _, _ = net.ParseCIDR(address2)
					address2 = addr.String()
		
					// Test the addresses do not match.
					Expect(address1).ToNot(Equal(address2))
		
					// Release addresses and the pool.
					err = am.ReleaseAddress(LocalDefaultAddressSpaceId, poolId, address1, nil)
					Expect(err).ToNot(HaveOccurred())
		
					err = am.ReleaseAddress(LocalDefaultAddressSpaceId, poolId, address2, nil)
					Expect(err).ToNot(HaveOccurred())
		
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolId)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})
)
