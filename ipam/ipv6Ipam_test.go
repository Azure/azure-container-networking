// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package ipam

import (
	"context"
	"reflect"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
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

func TestIPv6Ipam(t *testing.T) {
	options := make(map[string]interface{})
	options[common.OptEnvironment] = common.OptEnvironmentIPv6NodeIpam

	client := newKubernetesTestClient()
	node, _ := client.CoreV1().Nodes().Get(context.TODO(), testNodeName, metav1.GetOptions{})

	testInterfaces, err := retrieveKubernetesPodIPs(node, testSubnetSize)
	if err != nil {
		t.Fatalf("Failed to carve addresses: %+v", err)
	}

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

	if !equal {
		t.Fatalf("Network interface mismatch, expected: %+v actual: %+v", correctInterfaces, testInterfaces)
	}
}

func TestIPv6IpamWithoutIPv6SubnetAllocated(t *testing.T) {
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

	if err == nil {
		t.Fatal("Expected to fail IPv6 IP retrieval when node doesn't have IPv6 subnet")
	}
}
