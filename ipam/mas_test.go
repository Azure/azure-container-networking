// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net"
	"reflect"
	"runtime"
	"testing"
)

func TestMAS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MAS Suite")
}

var (
	_ = Describe("Test MAS", func() {

		Describe("Test create mas source", func() {
			Context("Create MAS with empty options", func() {
				It("Should return as default", func() {
					options := make(map[string]interface{})
					mas, _ := newMasSource(options)
					Expect(mas.name).Should(Equal("MAS"))
					if runtime.GOOS == windows {
						Expect(mas.filePath).Should(Equal(defaultWindowsFilePath))
					} else {
						Expect(mas.filePath).Should(Equal(defaultLinuxFilePath))
					}
				})
			})
		})

		Describe("Test GetSDNInterfaces", func() {

			const validFileName = "testfiles/masInterfaceConfig.json"
			const invalidFileName = "mas_test.go"
			const nonexistentFileName = "bad"

			Context("GetSDNInterfaces on interfaces", func() {
				It("interfaces should be equaled", func() {

					interfaces, err := getSDNInterfaces(validFileName)
					Expect(err).ShouldNot(HaveOccurred())

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

					equal := reflect.DeepEqual(interfaces, correctInterfaces)
					Expect(equal).To(BeTrue())
				})
			})

			Context("GetSDNInterfaces on invalid filename", func() {
				It("Should throw error on invalid filename", func() {
					interfaces, err := getSDNInterfaces(invalidFileName)
					if interfaces != nil {
						Expect(err).To(HaveOccurred())
					}
				})
			})

			Context("GetSDNInterfaces on nonexistent fileName", func() {
				It("Should throw error on nonexistent filename", func() {
					interfaces, err := getSDNInterfaces(nonexistentFileName)
					if interfaces != nil {
						Expect(err).To(HaveOccurred())
					}
				})
			})
		})

		Describe("Test PopulateAddressSpace", func() {

			Context("Simple interface", func() {
				It("Simple interface should run successfully", func() {

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
					Expect(err).ToNot(HaveOccurred())

					Expect(len(local.Pools)).To(Equal(1))

					pool, ok := local.Pools["1.0.0.0/12"]
					Expect(ok).To(BeTrue())

					Expect(pool.IfName).To(Equal("eth2"))
					Expect(pool.Priority).To(Equal(0))
					Expect(len(pool.Addresses)).To(Equal(2))

					_, ok = pool.Addresses["1.1.1.6"]
					Expect(ok).To(BeTrue())

					_, ok = pool.Addresses["1.1.1.7"]
					Expect(ok).To(BeTrue())
				})
			})

			Context("Multiple interface", func() {
				It("Multiple interface should run successfully", func() {

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
					Expect(err).ToNot(HaveOccurred())

					Expect(len(local.Pools)).To(Equal(4))

					pool, ok := local.Pools["0.0.0.0/24"]
					Expect(ok).To(BeTrue())
					Expect(pool.IfName).To(Equal("eth0"))
					Expect(pool.Priority).To(Equal(0))

					pool, ok = local.Pools["0.1.0.0/24"]
					Expect(ok).To(BeTrue())
					Expect(pool.IfName).To(Equal("eth0"))
					Expect(pool.Priority).To(Equal(0))

					pool, ok = local.Pools["1.0.0.0/24"]
					Expect(ok).To(BeTrue())
					Expect(pool.IfName).To(Equal("eth1"))
					Expect(pool.Priority).To(Equal(1))

					pool, ok = local.Pools["1.1.0.0/24"]
					Expect(ok).To(BeTrue())
					Expect(pool.IfName).To(Equal("eth1"))
					Expect(pool.Priority).To(Equal(1))
				})
			})
		})
	})
)
