// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/platform"
	"github.com/Azure/azure-container-networking/store"
	"github.com/Azure/azure-container-networking/testutils"
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

	if err := am.Initialize(&config, false, options); err != nil {
		return nil, err
	}

	if err := setupTestAddressSpace(am); err != nil {
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

	if err := amImpl.setAddressSpace(globalAs); err != nil {
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
	ap.newAddressRecord(&addr13)
	ap.newAddressRecord(&addr22)
	ap.newAddressRecord(&addr32)

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

	if err := amImpl.setAddressSpace(localAs); err != nil {
		return err
	}

	// Configure an empty global address space.
	globalAs, err := amImpl.newAddressSpace(GlobalDefaultAddressSpaceId, GlobalScope)
	if err != nil {
		return err
	}

	if err := amImpl.setAddressSpace(globalAs); err != nil {
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
					err = am.Initialize(&config, false,options)
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
					err = am.Initialize(&config, false,options)
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
					err = am.Initialize(&config, false, options)
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
					err = am.Initialize(&config, false,options)
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
					err := am.restore(false)
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
						Id:        "ap-test",
						RefCount:  1,
						Addresses: make(map[string]*addressRecord),
					}
					ap.Addresses["ar-test"] = &addressRecord{
						ID:    "ar-test",
						InUse: true,
					}
					as := &addressSpace{
						Id:    "as-test",
						Pools: make(map[string]*addressPool),
					}
					as.Pools["ap-test"] = ap
					am.AddrSpaces["as-test"] = as
					err := am.restore(false)
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
						Id:        "ap-test",
						RefCount:  1,
						Addresses: make(map[string]*addressRecord),
					}
					ap.Addresses["ar-test"] = &addressRecord{
						ID:    "ar-test",
						InUse: true,
					}
					as := &addressSpace{
						Id:    "as-test",
						Pools: make(map[string]*addressPool),
					}
					as.Pools["ap-test"] = ap
					am.AddrSpaces["as-test"] = as
					err := am.restore(false)
					Expect(err).To(BeNil())
					as = am.AddrSpaces["as-test"]
					ap = as.Pools["ap-test"]
					ar := ap.addrsByID["ar-test"]
					Expect(ar.ID).To(Equal("ar-test"))
					Expect(ap.RefCount).To(Equal(1))
					Expect(ar.InUse).To(BeTrue())
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
					am.AddrSpaces[LocalDefaultAddressSpaceId] = &addressSpace{Id: "localId"}
					am.AddrSpaces[GlobalDefaultAddressSpaceId] = &addressSpace{Id: "globalId"}
					localId, globalId := am.GetDefaultAddressSpaces()
					Expect(localId).To(Equal("localId"))
					Expect(globalId).To(Equal("globalId"))
				})
			})
		})
	})
)
