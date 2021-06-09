// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"context"
	"reflect"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"

	"github.com/Azure/azure-container-networking/common"
)

const (
	testNodeName   = "TestNode"
	testSubnetSize = "/126"
)

func newKubernetesTestClient() kubernetes.Interface {
	testnode := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNodeName,
		},
		Spec: v1.NodeSpec{
			PodCIDR:  "10.0.0.1/24",
			PodCIDRs: []string{"10.0.0.1/24", "ace:cab:deca:deed::/64"},
		},
	}

	client := testclient.NewSimpleClientset()
	client.CoreV1().Nodes().Create(context.TODO(), testnode, metav1.CreateOptions{})
	return client
}

func TestIpv6Ipam(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ipv6Ipam Suite")
}

var (
	_ = Describe("Test ipv6Ipam", func() {

		Describe("Test newIPv6IpamSource", func() {

			Context("When creating with current environment", func() {
				It("Should create successfully", func() {
					name := common.OptEnvironmentIPv6NodeIpam
					options := map[string]interface{}{}
					options[common.OptEnvironment] = name
					kubeConfigPath := defaultLinuxKubeConfigFilePath
					if runtime.GOOS == windows {
						kubeConfigPath = defaultWindowsKubeConfigFilePath
					}
					isLoaded := true
					ipv6IpamSource, err := newIPv6IpamSource(options, isLoaded)
					Expect(err).NotTo(HaveOccurred())
					Expect(ipv6IpamSource.name).To(Equal(name))
					Expect(ipv6IpamSource.kubeConfigPath).To(Equal(kubeConfigPath))
					Expect(ipv6IpamSource.isLoaded).To(Equal(isLoaded))
				})
			})
		})

		Describe("Test start and stop", func() {

			source := &ipv6IpamSource{}

			Context("Start the source with sink", func() {
				It("Should set the sink of source", func() {
					sink := &addressManagerMock{}
					err := source.start(sink)
					Expect(err).NotTo(HaveOccurred())
					Expect(source.sink).NotTo(BeNil())
				})
			})

			Context("Stop the source", func() {
				It("Should remove the sink of source", func() {
					source.stop()
					Expect(source.sink).To(BeNil())
				})
			})
		})

		Describe("TestIPv6Ipam", func() {
			Context("When node have IPv6 subnet", func() {
				It("Carve addresses successfully and Network interface match", func() {
					options := make(map[string]interface{})
					options[common.OptEnvironment] = common.OptEnvironmentIPv6NodeIpam

					client := newKubernetesTestClient()
					node, _ := client.CoreV1().Nodes().Get(context.TODO(), testNodeName, metav1.GetOptions{})

					testInterfaces, err := retrieveKubernetesPodIPs(node, testSubnetSize)
					Expect(err).NotTo(HaveOccurred())

					correctInterfaces := &NetworkInterfaces{
						Interfaces: []Interface{
							{
								IsPrimary: true,
								IPSubnets: []IPSubnet{
									{
										Prefix: "ace:cab:deca:deed::/126",
										IPAddresses: []IPAddress{
											{Address: "ace:cab:deca:deed::2", IsPrimary: false},
											{Address: "ace:cab:deca:deed::3", IsPrimary: false},
										},
									},
								},
							},
						},
					}

					equal := reflect.DeepEqual(testInterfaces, correctInterfaces)
					Expect(equal).To(BeTrue())
				})
			})

			Context("When node doesn't have IPv6 subnet", func() {
				It("Should fail to retrieve the IPv6 address", func() {
					options := make(map[string]interface{})
					options[common.OptEnvironment] = common.OptEnvironmentIPv6NodeIpam

					testnode := &v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: testNodeName,
						},
						Spec: v1.NodeSpec{
							PodCIDR:  "10.0.0.1/24",
							PodCIDRs: []string{"10.0.0.1/24"},
						},
					}

					_, err := retrieveKubernetesPodIPs(testnode, testSubnetSize)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})
)
