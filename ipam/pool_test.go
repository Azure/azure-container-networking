package ipam

import (
	"github.com/google/uuid"
	"net"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/testutils"
)

func TestPool(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pool Suite")
}

var (
	_ = Describe("Test Pool", func() {

		Describe("Test addressPoolId", func() {
			Context("Creates a new address pool ID object", func() {
				It("Should create a pool ID with given parameters", func() {
					asId := "eth0"
					subnet := "10.0.0.0/16"
					childSubnet := "10.0.1.0/8"
					apId := NewAddressPoolId(asId, subnet, childSubnet)
					Expect(apId.AsId).To(Equal(asId))
					Expect(apId.Subnet).To(Equal(subnet))
					Expect(apId.ChildSubnet).To(Equal(childSubnet))
				})
			})

			Context("Create a new pool ID when string format is incorrect", func() {
				It("Should raise an error", func() {
					s := "eth0|10.0.0.0/16|10.0.1.0/8|test"
					apId, err := NewAddressPoolIdFromString(s)
					Expect(apId).To(BeNil())
					Expect(err).To(HaveOccurred())
				})
			})

			Context("Create a new pool ID when string only contains addressSpace Id", func() {
				It("Should create a pool ID by parsing the string", func() {
					s := "local"
					apId, err := NewAddressPoolIdFromString(s)
					Expect(apId).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					Expect(apId.AsId).To(Equal(s))
					Expect(apId.Subnet).To(BeEmpty())
					Expect(apId.ChildSubnet).To(BeEmpty())
				})
			})

			Context("Create a new pool ID when the string has addrspace and subnet", func() {
				It("Should create a pool ID by parsing the string", func() {
					s := "eth0|10.0.0.0/16"
					apId, err := NewAddressPoolIdFromString(s)
					Expect(apId).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					Expect(apId.AsId).To(Equal("eth0"))
					Expect(apId.Subnet).To(Equal("10.0.0.0/16"))
					Expect(apId.ChildSubnet).To(BeEmpty())
				})
			})

			Context("Create a new pool ID when string has addrspace, subnet and child subnet", func() {
				It("Should create a pool ID by parsing the string", func() {
					s := "eth0|10.0.0.0/16|10.0.1.0/8"
					apId, err := NewAddressPoolIdFromString(s)
					Expect(apId).NotTo(BeNil())
					Expect(err).NotTo(HaveOccurred())
					Expect(apId.AsId).To(Equal("eth0"))
					Expect(apId.Subnet).To(Equal("10.0.0.0/16"))
					Expect(apId.ChildSubnet).To(Equal("10.0.1.0/8"))
				})
			})

			Context("Returns the string representation of a pool ID with childSubnet", func() {
				It("Should return string with asID|subnet|childsubnet", func() {
					apId := &addressPoolId{
						AsId:        "eth0",
						Subnet:      "10.0.0.0/16",
						ChildSubnet: "10.0.1.0/8",
					}
					s := apId.String()
					Expect(s).To(Equal("eth0|10.0.0.0/16|10.0.1.0/8"))
				})
			})

			Context("Returns the string representation of a pool ID without childSubnet", func() {
				It("Should return a string without childSubnet", func() {
					apId := &addressPoolId{
						AsId:   "eth0",
						Subnet: "10.0.0.0/16",
					}
					s := apId.String()
					Expect(s).To(Equal("eth0|10.0.0.0/16"))
				})
			})
		})

		Describe("Test newAddressSpace", func() {

			am := &addressManager{}

			Context("When scope is LocalScope", func() {
				It("Should create an addressSpace with LocalScope", func() {
					asId := "local"
					scope := LocalScope
					as, err := am.newAddressSpace(asId, scope)
					Expect(err).NotTo(HaveOccurred())
					Expect(as.Id).To(Equal(asId))
					Expect(as.Scope).To(Equal(scope))
				})
			})

			Context("When scope is GlobalScope", func() {
				It("Should create an addressSpace with GlobalScope", func() {
					asId := "local"
					scope := GlobalScope
					as, err := am.newAddressSpace(asId, scope)
					Expect(err).NotTo(HaveOccurred())
					Expect(as.Id).To(Equal(asId))
					Expect(as.Scope).To(Equal(scope))
				})
			})

			Context("When scope is not GlobalScope or LocalScope", func() {
				It("Should raise an error", func() {
					asId := "local"
					scope := 127
					as, err := am.newAddressSpace(asId, scope)
					Expect(err).To(HaveOccurred())
					Expect(as).To(BeNil())
				})
			})
		})

		Describe("Test getAddressSpace and setAddressSpace", func() {

			am := &addressManager{
				AddrSpaces: map[string]*addressSpace{},
			}

			Context("When addressSpace not exists", func() {
				It("Should raise an error", func() {
					as, err := am.getAddressSpace("lo")
					Expect(err).To(Equal(errInvalidAddressSpace))
					Expect(as).To(BeNil())
				})
			})

			Context("When addressSpace exists", func() {
				It("Should return the addressSpace", func() {
					asId := "local"
					am.AddrSpaces[asId] = &addressSpace{Id: asId}
					as, err := am.getAddressSpace(asId)
					Expect(err).NotTo(HaveOccurred())
					Expect(as.Id).To(Equal(asId))
				})
			})
		})

		Describe("Test setAddressSpace", func() {

			am := &addressManager{
				AddrSpaces: map[string]*addressSpace{},
			}
			am.netApi = &testutils.NetApiMock{}
			asId := "local"

			Context("When addressSpace not exists", func() {
				It("Should be added to the am", func() {
					err := am.setAddressSpace(&addressSpace{Id: asId})
					Expect(err).NotTo(HaveOccurred())
					Expect(am.AddrSpaces[asId].Id).To(Equal(asId))
				})
			})

			Context("When addressSpace already exists", func() {
				It("Should be merged", func() {
					err := am.setAddressSpace(&addressSpace{Id: asId})
					Expect(err).NotTo(HaveOccurred())
					Expect(am.AddrSpaces[asId].Id).To(Equal(asId))
				})
			})
		})

		Describe("Test merge", func() {
			Context("When only new addressSpace contains the pool", func() {
				It("The pool should be merged to the origin addressSpace", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"
					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					pool := &addressPool{
						Id: poolId,
						as: originAs,
					}
					newAs.Pools[poolId] = pool
					originAs.merge(newAs)
					pool = originAs.Pools[poolId]
					Expect(pool.as.Id).To(Equal(asId))
					Expect(pool.epoch).To(Equal(4))
					Expect(newAs.Pools[poolId]).To(BeNil())
				})
			})

			Context("When only new addressSpace contains the addressRecord", func() {
				It("The addressRecord should be merged to the origin addressSpace", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"
					arId := "10.0.0.1/16"

					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					pool1 := &addressPool{
						Id:        poolId,
						as:        originAs,
						Addresses: map[string]*addressRecord{},
					}
					originAs.Pools[poolId] = pool1

					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					pool2 := &addressPool{
						Id:        poolId,
						as:        newAs,
						Addresses: map[string]*addressRecord{},
					}
					pool2.Addresses[arId] = &addressRecord{InUse: true}
					newAs.Pools[poolId] = pool2
					originAs.merge(newAs)
					pool1 = originAs.Pools[poolId]
					Expect(pool1.as.Id).To(Equal(asId))
					ar := pool1.Addresses[arId]
					Expect(ar.epoch).To(Equal(4))
					Expect(ar.InUse).To(BeTrue())
					Expect(newAs.Pools[poolId]).To(BeNil())
				})
			})

			Context("When addressRecord is contained in both new addressSpace and origin addressSpace", func() {
				It("The addressRecord of origin addressSpace should be updated", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"
					arId := "10.0.0.1/16"

					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					pool1 := &addressPool{
						Id:        poolId,
						as:        originAs,
						Addresses: map[string]*addressRecord{},
					}
					pool1.Addresses[arId] = &addressRecord{InUse: true, unhealthy: true}
					originAs.Pools[poolId] = pool1

					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					pool2 := &addressPool{
						Id:        poolId,
						as:        newAs,
						Addresses: map[string]*addressRecord{},
					}
					pool2.Addresses[arId] = &addressRecord{InUse: true}
					newAs.Pools[poolId] = pool2
					originAs.merge(newAs)
					pool1 = originAs.Pools[poolId]
					Expect(pool1.as.Id).To(Equal(asId))
					ar := pool1.Addresses[arId]
					Expect(ar.epoch).To(Equal(4))
					Expect(ar.InUse).To(BeTrue())
					Expect(ar.unhealthy).To(BeFalse())
					Expect(newAs.Pools[poolId]).To(BeNil())
				})
			})

			Context("When addressRecord epoch is correct and pool epoch is less", func() {
				It("Should update the pool epoch", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"
					arId := "10.0.0.1/16"

					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					pool1 := &addressPool{
						Id:        poolId,
						as:        originAs,
						epoch:     3,
						Addresses: map[string]*addressRecord{},
					}
					pool1.Addresses[arId] = &addressRecord{
						epoch: 4,
						InUse: true,
					}
					originAs.Pools[poolId] = pool1
					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					originAs.merge(newAs)
					pool1 = originAs.Pools[poolId]
					Expect(pool1.epoch).To(Equal(4))
					ar := pool1.Addresses[arId]
					Expect(ar.epoch).To(Equal(4))
					Expect(ar.InUse).To(BeTrue())
					Expect(ar.unhealthy).To(BeFalse())
				})
			})

			Context("When addressRecord is in use", func() {
				It("The addressRecord should be set to unhealthy", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"
					arId := "10.0.0.1/16"

					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					pool1 := &addressPool{
						Id:        poolId,
						as:        originAs,
						epoch:     3,
						Addresses: map[string]*addressRecord{},
					}
					pool1.Addresses[arId] = &addressRecord{
						epoch: 3,
						InUse: true,
					}
					originAs.Pools[poolId] = pool1
					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					originAs.merge(newAs)
					pool1 = originAs.Pools[poolId]
					Expect(pool1.epoch).To(Equal(4))
					ar := pool1.Addresses[arId]
					Expect(ar.epoch).To(Equal(3))
					Expect(ar.InUse).To(BeTrue())
					Expect(ar.unhealthy).To(BeTrue())
				})
			})

			Context("When addressRecord is not in use but pool is in use", func() {
				It("The addressRecord should be deleted", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"
					arId := "10.0.0.1/16"

					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					pool := &addressPool{
						Id:        poolId,
						as:        originAs,
						epoch:     3,
						RefCount:  1,
						Addresses: map[string]*addressRecord{},
					}
					pool.Addresses[arId] = &addressRecord{
						epoch: 3,
						InUse: false,
					}
					originAs.Pools[poolId] = pool
					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					originAs.merge(newAs)
					pool = originAs.Pools[poolId]
					Expect(pool.epoch).To(Equal(3))
					ar := pool.Addresses[arId]
					Expect(ar).To(BeNil())
				})
			})

			Context("When pool is not in use", func() {
				It("The pool should be deleted", func() {
					asId := "local"
					epoch := 3
					poolId := "10.0.0.0/16"

					originAs := &addressSpace{
						Id:    asId,
						epoch: epoch,
						Pools: map[string]*addressPool{},
					}
					pool := &addressPool{
						Id:        poolId,
						as:        originAs,
						epoch:     3,
						RefCount:  0,
						Addresses: map[string]*addressRecord{},
					}
					originAs.Pools[poolId] = pool
					newAs := &addressSpace{
						Id:    asId,
						Pools: map[string]*addressPool{},
					}
					originAs.merge(newAs)
					pool = originAs.Pools[poolId]
					Expect(pool).To(BeNil())
				})
			})
		})

		Describe("Test newAddressPool", func() {
			Context("When pool already exists", func() {
				It("Should raise an error", func() {
					subnet := &net.IPNet{
						IP:   net.IPv4(10, 0, 0, 1),
						Mask: net.IPv4Mask(255, 255, 0, 0),
					}
					poolId := subnet.String()
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					as.Pools[poolId] = &addressPool{Id: poolId}
					pool, err := as.newAddressPool("", 0, subnet)
					Expect(err).To(Equal(errAddressPoolExists))
					Expect(pool.Id).To(Equal(poolId))
				})
			})

			Context("When pool not exists", func() {
				It("Should create pool successfully", func() {
					subnet := &net.IPNet{
						IP:   net.IPv4(10, 0, 0, 1),
						Mask: net.IPv4Mask(255, 255, 0, 0),
					}
					poolId := subnet.String()
					as := &addressSpace{
						Id:    "local",
						Pools: map[string]*addressPool{},
					}
					pool, err := as.newAddressPool("local", 1, subnet)
					Expect(err).NotTo(HaveOccurred())
					Expect(pool.Id).To(Equal(poolId))
					Expect(pool.as.Id).To(Equal(as.Id))
					Expect(pool.IfName).To(Equal("local"))
					Expect(pool.Subnet.String()).To(Equal(poolId))
					Expect(pool.IsIPv6).To(BeFalse())
					Expect(pool.Priority).To(Equal(1))
					Expect(as.Pools[poolId]).NotTo(BeNil())
				})
			})

			Context("When pool is ipv6", func() {
				It("Should create pool successfully", func() {
					subnet := &net.IPNet{
						IP:   net.IPv6zero,
						Mask: net.IPv6zero.DefaultMask(),
					}
					poolId := subnet.String()
					as := &addressSpace{
						Id:    "local",
						Pools: map[string]*addressPool{},
					}
					pool, err := as.newAddressPool("local", 1, subnet)
					Expect(err).NotTo(HaveOccurred())
					Expect(pool.Id).To(Equal(poolId))
					Expect(pool.as.Id).To(Equal(as.Id))
					Expect(pool.IfName).To(Equal("local"))
					Expect(pool.Subnet.String()).To(Equal(poolId))
					Expect(pool.IsIPv6).To(BeTrue())
					Expect(pool.Priority).To(Equal(1))
					Expect(as.Pools[poolId]).NotTo(BeNil())
				})
			})
		})

		Describe("Test getAddressPool", func() {
			Context("When pool not find", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					pool, _ := as.getAddressPool("10.0.0.0/16")
					Expect(pool).To(BeNil())
				})
			})

			Context("When pool is found", func() {
				It("Should return the pool", func() {
					poolId := "10.0.0.0/16"
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					as.Pools[poolId] = &addressPool{Id: poolId}
					pool, err := as.getAddressPool(poolId)
					Expect(err).NotTo(HaveOccurred())
					Expect(pool.Id).To(Equal(poolId))
				})
			})
		})

		Describe("Test requestPool", func() {
			Context("When poolId is explicitly specified and not found in addressSpace", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					ap, err := as.requestPool("10.0.0.0/16", "", nil, false)
					Expect(err).To(Equal(errAddressPoolNotFound))
					Expect(ap).To(BeNil())
				})
			})

			Context("When poolId is explicitly specified and found in addressSpace", func() {
				It("Should return the pool", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					as.Pools[poolId] = &addressPool{
						Id:       poolId,
						RefCount: 0,
					}
					ap, err := as.requestPool(poolId, "", nil, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Id).To(Equal(poolId))
					Expect(ap.RefCount).To(Equal(1))
				})
			})

			Context("When pool is in use and it has no ips allocated", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					as.Pools[poolId] = &addressPool{
						Id:        poolId,
						RefCount:  1,
						Addresses: map[string]*addressRecord{},
					}
					as.Pools[poolId].Addresses["10.0.0.2"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}
					ap, err := as.requestPool("", "", nil, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Id).To(Equal(poolId))
					Expect(ap.RefCount).To(Equal(1))
				})
			})

			Context("When pool is in use and it has ips allocated", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					as.Pools[poolId] = &addressPool{
						Id:        poolId,
						RefCount:  1,
						Addresses: map[string]*addressRecord{},
					}
					as.Pools[poolId].Addresses["10.0.0.1"] = &addressRecord{
						InUse: true,
						Addr:  net.IPv4zero,
					}
					as.Pools[poolId].Addresses["10.0.0.2"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}
					ap, err := as.requestPool("", "", nil, false)
					Expect(err).To(HaveOccurred())
					Expect(ap).To(BeNil())
					Expect(err).To(Equal(errNoAvailableAddressPools))
				})
			})

			Context("When pool is in use and request same pool explicitly", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					as.Pools[poolId] = &addressPool{
						Id:        poolId,
						RefCount:  1,
						Addresses: map[string]*addressRecord{},
					}
					as.Pools[poolId].Addresses["10.0.0.1"] = &addressRecord{
						InUse: true,
						Addr:  net.IPv4zero,
					}
					as.Pools[poolId].Addresses["10.0.0.2"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}
					ap, err := as.requestPool(poolId, "", nil, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap).NotTo(BeNil())
					Expect(ap.RefCount).To(Equal(1))
				})
			})

			Context("When pool is ipv4 and ipv6 is wanted", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					as.Pools[poolId] = &addressPool{
						Id:     poolId,
						IsIPv6: false,
					}
					ap, err := as.requestPool("", "", nil, true)
					Expect(err).To(Equal(errNoAvailableAddressPools))
					Expect(ap).To(BeNil())
				})
			})

			Context("When the requested interface name does not exist", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					ifName := "local"
					as.Pools[poolId] = &addressPool{
						Id:     poolId,
						IfName: ifName,
					}
					options := map[string]string{}
					options[OptInterfaceName] = "en0"
					ap, err := as.requestPool("", "", options, false)
					Expect(err).To(Equal(errNoAvailableAddressPools))
					Expect(ap).To(BeNil())
				})
			})

			Context("When addressSpace has one pool available", func() {
				It("Should return the pool", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					poolId := "10.0.0.0/16"
					as.Pools[poolId] = &addressPool{
						Id: poolId,
					}
					ap, err := as.requestPool("", "", nil, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Id).To(Equal(poolId))
					Expect(ap.RefCount).To(Equal(1))
				})
			})

			Context("When addressSpace has pools with different priority", func() {
				It("Should return the pool with the highest priority", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					as.Pools["10.0.0.0/16"] = &addressPool{
						Id:       "10.0.0.0/16",
						Priority: 1,
					}
					as.Pools["10.1.0.0/16"] = &addressPool{
						Id:       "10.1.0.0/16",
						Priority: 2,
					}
					ap, err := as.requestPool("", "", nil, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Id).To(Equal("10.1.0.0/16"))
					Expect(ap.RefCount).To(Equal(1))
				})
			})

			Context("When addressSpace has pools with different addresses", func() {
				It("Should return the pool with the highest number of addresses", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					as.Pools["10.0.0.0/16"] = &addressPool{
						Id:        "10.0.0.0/16",
						Addresses: map[string]*addressRecord{},
					}
					as.Pools["10.1.0.0/16"] = &addressPool{
						Id: "10.1.0.0/16",
						Addresses: map[string]*addressRecord{
							"10.1.0.1/16": &addressRecord{},
						},
					}
					ap, err := as.requestPool("", "", nil, false)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Id).To(Equal("10.1.0.0/16"))
					Expect(ap.RefCount).To(Equal(1))
				})
			})
		})

		Describe("Test releasePool", func() {
			Context("When pool not found", func() {
				It("Should raise an error", func() {
					as := &addressSpace{
						Pools: map[string]*addressPool{},
					}
					err := as.releasePool("10.0.0.0/16")
					Expect(err).To(Equal(errAddressPoolNotFound))
				})
			})

			Context("When pool's addresses are all not in use", func() {
				It("Should release the pool ", func() {
					poolId := "10.0.0.0/16"
					as := &addressSpace{
						epoch: 2,
						Pools: map[string]*addressPool{},
					}
					as.Pools[poolId] = &addressPool{
						epoch:     1,
						RefCount:  1,
						Addresses: map[string]*addressRecord{},
					}

					as.Pools[poolId].Addresses["10.0.0.1"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}
					as.Pools[poolId].Addresses["10.0.0.2"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}
					as.Pools[poolId].Addresses["10.0.0.3"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}

					err := as.releasePool("10.0.0.0/16")
					Expect(err).NotTo(HaveOccurred())
					Expect(as.Pools[poolId].isInUse()).To(BeFalse())
				})
			})

			Context("When the pool has in use addresses", func() {
				It("Should not delete the pool", func() {
					poolId := "10.0.0.0/16"
					as := &addressSpace{
						epoch: 1,
						Pools: map[string]*addressPool{},
					}
					as.Pools[poolId] = &addressPool{
						epoch:     1,
						RefCount:  1,
						Addresses: map[string]*addressRecord{},
					}

					as.Pools[poolId].Addresses["10.0.0.1"] = &addressRecord{
						InUse: true,
						Addr:  net.IPv4zero,
					}
					as.Pools[poolId].Addresses["10.0.0.2"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}
					as.Pools[poolId].Addresses["10.0.0.3"] = &addressRecord{
						InUse: false,
						Addr:  net.IPv4zero,
					}

					err := as.releasePool("10.0.0.0/16")
					Expect(err).NotTo(HaveOccurred())
					Expect(as.Pools[poolId]).NotTo(BeNil())
				})
			})

			Context("When pool is still in use", func() {
				It("Should not delete the pool", func() {
					poolId := "10.0.0.0/16"
					as := &addressSpace{
						epoch: 2,
						Pools: map[string]*addressPool{},
					}
					as.Pools[poolId] = &addressPool{
						epoch:    1,
						RefCount: 2,
					}
					err := as.releasePool("10.0.0.0/16")
					Expect(err).NotTo(HaveOccurred())
					Expect(as.Pools[poolId]).NotTo(BeNil())
				})
			})
		})

		Describe("Test getInfo", func() {
			Context("When addressRecord is not in use", func() {
				It("Should add the available", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					ap.Addresses["10.0.0.1/16"] = &addressRecord{InUse: false}
					ap.Addresses["10.0.0.2/16"] = &addressRecord{InUse: true}
					ap.Addresses["10.0.0.3/16"] = &addressRecord{InUse: false}
					apInfo := ap.getInfo()
					Expect(apInfo.Available).To(Equal(2))
				})
			})

			Context("When addressRecords are unhealthy", func() {
				It("Should append the unhealthyAddrs", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					ap.Addresses["10.0.0.1/16"] = &addressRecord{
						unhealthy: true,
						Addr:      net.IPv4zero,
					}
					ap.Addresses["10.0.0.2/16"] = &addressRecord{
						unhealthy: false,
						Addr:      net.IPv4zero,
					}
					ap.Addresses["10.0.0.3/16"] = &addressRecord{
						unhealthy: true,
						Addr:      net.IPv4zero,
					}
					apInfo := ap.getInfo()
					Expect(len(apInfo.UnhealthyAddrs)).To(Equal(2))
				})
			})
		})

		Describe("Test isInUse", func() {
			Context("When RefCount is set to some value", func() {
				It("Should return true when RefCount > 0", func() {
					ap := &addressPool{RefCount: 0}
					Expect(ap.isInUse()).To(BeFalse())
					ap.RefCount = 1
					Expect(ap.isInUse()).To(BeTrue())
					ap.RefCount = 10000
					Expect(ap.isInUse()).To(BeTrue())
					ap.RefCount = -1
					Expect(ap.isInUse()).To(BeFalse())
				})
			})
		})

		Describe("Test requestAddress", func() {
			Context("When addressRecord not found", func() {
				It("Should raise errAddressNotFound", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					addr, err := ap.requestAddress("10.0.0.1/16", nil)
					Expect(err).To(Equal(errAddressNotFound))
					Expect(addr).To(BeEmpty())
				})
			})

			Context("When addressRecord is in use and id match", func() {
				It("Should return the same addressRecord", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
						addrsByID: map[string]*addressRecord{},
					}
					arId := "10.0.0.1/16"
					ap.Addresses[arId] = &addressRecord{
						ID:    arId,
						InUse: true,
					}
					options := map[string]string{}
					options[OptAddressID] = arId
					addr, err := ap.requestAddress("10.0.0.1/16", options)
					Expect(err).NotTo(HaveOccurred())
					Expect(addr).NotTo(BeEmpty())
					Expect(ap.addrsByID[arId].ID).To(Equal(arId))
				})
			})

			Context("When id is not found and a address is available", func() {
				It("Should return a new address", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
						addrsByID: map[string]*addressRecord{},
						Subnet:    subnet1,
					}
					arId := uuid.New().String()

					ap.Addresses["0"] = &addressRecord{
						ID:    "",
						Addr:  addr11,
						InUse: false,
					}

					ap.Addresses["1"] = &addressRecord{
						ID:    "",
						Addr:  addr12,
						InUse: false,
					}

					ap.Addresses["3"] = &addressRecord{
						ID:    "",
						Addr:  addr13,
						InUse: false,
					}

					options := map[string]string{}
					options[OptAddressID] = arId

					addr, err := ap.requestAddress("", options)
					Expect(err).NotTo(HaveOccurred())
					Expect(addr).NotTo(BeEmpty())
					Expect(ap.addrsByID[arId].ID).To(Equal(arId))
				})
			})

			Context("When addressRecord is in use and id is empty", func() {
				It("Should raise errAddressInUse", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					ap.Addresses["10.0.0.1/16"] = &addressRecord{InUse: true}
					addr, err := ap.requestAddress("10.0.0.1/16", nil)
					Expect(err).To(Equal(errAddressInUse))
					Expect(addr).To(BeEmpty())
				})
			})

			Context("When addressRecord is in use and id is not equal to the addressRecord's id", func() {
				It("Should raise errAddressInUse", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					arId := "10.0.0.1/16"
					ap.Addresses[arId] = &addressRecord{
						ID:    arId,
						InUse: true,
					}
					options := map[string]string{}
					options[OptAddressID] = "10.0.0.2/16"
					addr, err := ap.requestAddress("10.0.0.1/16", options)
					Expect(err).To(Equal(errAddressInUse))
					Expect(addr).To(BeEmpty())
				})
			})

			Context("When OptAddressType is OptAddressTypeGateway", func() {
				It("Should raise errAddressInUse", func() {
					ap := &addressPool{
						Gateway: net.IPv4(10, 0, 0, 1),
					}
					options := map[string]string{}
					options[OptAddressType] = OptAddressTypeGateway
					addr, err := ap.requestAddress("", options)
					Expect(err).NotTo(HaveOccurred())
					Expect(addr).NotTo(BeEmpty())
				})
			})

			Context("When id match", func() {
				It("Should return the addressRecord with given id", func() {
					ap := &addressPool{
						addrsByID: map[string]*addressRecord{},
					}
					arId := "10.0.0.1/16"
					ap.addrsByID[arId] = &addressRecord{
						ID:    arId,
						InUse: true,
					}
					options := map[string]string{}
					options[OptAddressID] = arId
					addr, err := ap.requestAddress("", options)
					Expect(err).NotTo(HaveOccurred())
					Expect(addr).NotTo(BeEmpty())
					Expect(ap.addrsByID[arId].ID).To(Equal(arId))
				})
			})

			Context("When no available address", func() {
				It("Should raise errNoAvailableAddresses", func() {
					ap := &addressPool{
						addrsByID: map[string]*addressRecord{},
					}
					// ar.InUse is true
					ap.addrsByID["10.0.0.1/16"] = &addressRecord{
						ID:    "",
						InUse: true,
					}
					// ad.Id != ""
					ap.addrsByID["10.0.0.2/16"] = &addressRecord{
						ID:    "10.0.0.2/16",
						InUse: false,
					}
					addr, err := ap.requestAddress("", nil)
					Expect(err).To(Equal(errNoAvailableAddresses))
					Expect(addr).To(BeEmpty())
				})
			})

			Context("Check if the InUse is set to true", func() {
				It("InUse should be set to true", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					arId := "10.0.0.1/16"
					ap.Addresses[arId] = &addressRecord{
						InUse: false,
					}
					addr, err := ap.requestAddress("", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(addr).NotTo(BeEmpty())
					Expect(ap.Addresses[arId].InUse).To(BeTrue())
				})
			})
		})

		Describe("Test releaseAddress", func() {
			Context("When address is equal to the gateway", func() {
				It("Should return nil", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
						Gateway:   net.IPv4zero,
					}
					ar := ap.Gateway.String()
					err := ap.releaseAddress(ar, nil)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("When address is equal to the gateway", func() {
				It("Should return nil", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
						Gateway:   net.IPv4zero,
					}
					ar := ap.Gateway.String()
					err := ap.releaseAddress(ar, nil)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("When addr is not in use", func() {
				It("Should return nil", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
					}
					ap.Addresses["10.0.0.1/16"] = &addressRecord{InUse: false}
					err := ap.releaseAddress("10.0.0.1/16", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Addresses["10.0.0.1/16"]).NotTo(BeNil())
				})
			})

			Context("When delete by id", func() {
				It("Should return nil", func() {
					ap := &addressPool{
						addrsByID: map[string]*addressRecord{},
						Addresses: map[string]*addressRecord{},
						as:        &addressSpace{epoch: 1},
					}
					arId := "10.0.0.1/16"
					ap.addrsByID[arId] = &addressRecord{
						ID:    arId,
						Addr:  net.IPv4zero,
						InUse: true,
						epoch: 1,
					}
					options := map[string]string{}
					options[OptAddressID] = arId
					err := ap.releaseAddress("", options)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.addrsByID[arId]).To(BeNil())
				})
			})

			Context("When delete by address", func() {
				It("Should return nil", func() {
					ap := &addressPool{
						Addresses: map[string]*addressRecord{},
						as:        &addressSpace{epoch: 2},
					}
					ap.Addresses["10.0.0.1/16"] = &addressRecord{
						InUse: true,
						epoch: 1,
					}
					err := ap.releaseAddress("10.0.0.1/16", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(ap.Addresses["10.0.0.1/16"]).To(BeNil())
				})
			})
		})
	})
)
