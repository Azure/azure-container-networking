// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/Azure/azure-container-networking/common"
)

var (
	// Pools and addresses used by tests.
	ipv6subnet1 = "ace:cab:deca:deed::" + testSubnetSize
	ipv6addr2   = "ace:cab:deca:deed::2"
	ipv6addr3   = "ace:cab:deca:deed::3"
)

func TestManagerIpv6Ipam(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manager ipv6ipam Suite")
}

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

var (
	_ = Describe("Test manager ipv6Ipam", func() {

		Describe("Test IPv6 get address pool and address", func() {

			var (
				am       AddressManager
				err      error
				poolID1  string
				subnet1  string
				address2 string
				address3 string
			)

			Context("Start with the test address space", func() {
				It("Should create AddressManager successfully", func() {
					am, err = createTestIpv6AddressManager()
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("When test if the address spaces are returned correctly", func() {
				It("GetDefaultAddressSpaces returned valid local address space", func() {
					local, _ := am.GetDefaultAddressSpaces()
					Expect(local).To(Equal(LocalDefaultAddressSpaceId))
				})
			})

			Context("When request two separate address pools", func() {
				It("Should request pool successfully and return subnet matched ipv6subnet1", func() {
					poolID1, subnet1, err = am.RequestPool(LocalDefaultAddressSpaceId, "", "", nil, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(subnet1).To(Equal(ipv6subnet1))

				})
			})

			Context("When test with a specified address", func() {
				It("Should request address successfully", func() {
					address2, err := am.RequestAddress(LocalDefaultAddressSpaceId, poolID1, ipv6addr2, nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(address2).To(Equal(ipv6addr2 + testSubnetSize))
				})
			})

			Context("When test without requesting address explicitly", func() {
				It("Should request address successfully", func() {
					address3, err := am.RequestAddress(LocalDefaultAddressSpaceId, poolID1, "", nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(address3).To(Equal(ipv6addr3 + testSubnetSize))
				})
			})

			Context("When release address2", func() {
				It("Should release successfully", func() {
					err = am.ReleaseAddress(LocalDefaultAddressSpaceId, poolID1, address2, nil)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("When release address3 and the pool", func() {
				It("Should release successfully", func() {
					err = am.ReleaseAddress(LocalDefaultAddressSpaceId, poolID1, address3, nil)
					Expect(err).NotTo(HaveOccurred())
					err = am.ReleasePool(LocalDefaultAddressSpaceId, poolID1)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	})
)
