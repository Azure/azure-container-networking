package debug

import (
	"log"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	NPMIPtable "github.com/Azure/azure-container-networking/npm/pkg/dataplane/iptables"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestGetProtobufRulesFromIptableFile(t *testing.T) {
	c := &Converter{}
	_, err := c.GetProtobufRulesFromIptableFile(
		util.IptablesFilterTable,
		npmCacheFileV1,
		iptableSaveFileV1,
	)
	if err != nil {
		t.Errorf("error during TestGetJSONRulesFromIptable : %v", err)
	}
}

func TestGetProtobufRulesFromIptableFileV2(t *testing.T) {
	c := &Converter{
		EnableV2NPM: true,
	}
	rules, err := c.GetProtobufRulesFromIptableFile(
		util.IptablesFilterTable,
		npmCacheFileV2,
		iptableSaveFileV2,
	)
	require.NoError(t, err)
	log.Printf("rules %+v", rules)

	srcPod := &common.NpmPod{
		Name:      "a",
		Namespace: "y",
		PodIP:     "10.224.0.70",
		Labels: map[string]string{
			"pod": "a",
		},
		ContainerPorts: []v1.ContainerPort{
			{
				Name:          "serve-80-tcp",
				ContainerPort: 80,
				Protocol:      "TCP",
			},
			{
				Name:          "serve-80-udp",
				ContainerPort: 80,
				Protocol:      "UDP",
			},
			{
				Name:          "serve-81-tcp",
				ContainerPort: 81,
				Protocol:      "TCP",
			},
			{
				Name:          "serve-81-UDP",
				ContainerPort: 81,
				Protocol:      "UDP",
			},
		},
	}

	dstPod := &common.NpmPod{
		Name:      "a",
		Namespace: "y",
		PodIP:     "10.224.0.70",
		Labels: map[string]string{
			"pod": "a",
		},
		ContainerPorts: []v1.ContainerPort{
			{
				Name:          "serve-80-tcp",
				ContainerPort: 80,
				Protocol:      "TCP",
			},
			{
				Name:          "serve-80-udp",
				ContainerPort: 80,
				Protocol:      "UDP",
			},
			{
				Name:          "serve-81-tcp",
				ContainerPort: 81,
				Protocol:      "TCP",
			},
			{
				Name:          "serve-81-UDP",
				ContainerPort: 81,
				Protocol:      "UDP",
			},
		},
	}

	hitrules, _, _, err := getHitRules(srcPod, dstPod, rules, c.NPMCache)
	require.NoError(t, err)
	log.Printf("hitrules %+v", hitrules)
	if err != nil {
		t.Errorf("failed to test GetJSONRulesFromIptable : %v", err)
	}
}

func TestNpmCacheFromFile(t *testing.T) {
	c := &Converter{
		EnableV2NPM: true,
	}
	err := c.NpmCacheFromFile(npmCacheFileV2)
	if err != nil {
		t.Errorf("Failed to decode NPMCache from %s file : %v", npmCacheFileV1, err)
	}
}

func TestGetSetType(t *testing.T) {
	tests := map[string]struct {
		inputSetName string
		inputMapName string
		expected     pb.SetType
	}{
		"namespace": {
			inputSetName: "ns-testnamespace",
			inputMapName: "SetMap",
			expected:     pb.SetType_NAMESPACE,
		},
		"key value label of pod": {
			inputSetName: "app:frontend",
			inputMapName: "SetMap",
			expected:     pb.SetType_KEYVALUELABELOFPOD,
		},
		"nested label of pod": {
			inputSetName: "k1:v0:v1",
			inputMapName: "ListMap",
			expected:     pb.SetType_NESTEDLABELOFPOD,
		},
		"key label of namespace": {
			inputSetName: "all-namespaces",
			inputMapName: "ListMap",
			expected:     pb.SetType_KEYLABELOFNAMESPACE,
		},
		"namedports": {
			inputSetName: "namedport:serve-80",
			inputMapName: "SetMap",
			expected:     pb.SetType_NAMEDPORTS,
		},
		"key label of pod": {
			inputSetName: "k0",
			inputMapName: "SetMap",
			expected:     pb.SetType_KEYLABELOFPOD,
		},
		"key value label of namespace": {
			inputSetName: "ns-namespace:test0",
			inputMapName: "ListMap",
			expected:     pb.SetType_KEYVALUELABELOFNAMESPACE,
		},
		"CIDRBlock": {
			inputSetName: "k8s-example-policy-in-ns-default-0in",
			inputMapName: "SetMap",
			expected:     pb.SetType_CIDRBLOCKS,
		},
	}

	c := &Converter{
		EnableV2NPM: true,
	}
	err := c.initConverterFile(npmCacheFileV2)
	if err != nil {
		t.Errorf("error during initilizing converter : %v", err)
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			actualType := c.getSetType(test.inputSetName, test.inputMapName)
			diff := cmp.Diff(test.expected, actualType)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}

func TestGetRulesFromChain(t *testing.T) {
	type test struct {
		input    *NPMIPtable.Chain
		expected []*pb.RuleResponse
	}

	rawchain := &NPMIPtable.Chain{
		Name: "AZURE-NPM-EGRESS",
		Rules: []*NPMIPtable.Rule{
			{
				Protocol: "",
				Target: &NPMIPtable.Target{
					Name:           "AZURE-NPM-EGRESS-2697641196",
					OptionValueMap: map[string][]string{},
				},
				Modules: []*NPMIPtable.Module{
					{
						Verb:           "set",
						OptionValueMap: map[string][]string{"match-set": {"azure-npm-3922407721", "src"}},
					},
					{
						Verb:           "set",
						OptionValueMap: map[string][]string{"match-set": {"azure-npm-2837910840", "src"}},
					},
					{
						Verb:           "comment",
						OptionValueMap: map[string][]string{"comment": {"EGRESS-POLICY-y/base-FROM-podlabel-pod:a-AND-ns-y-IN-ns-y"}},
					},
				},
			},
		},
	}

	expected := []*pb.RuleResponse{{
		Chain: "AZURE-NPM-EGRESS",
		SrcList: []*pb.RuleResponse_SetInfo{
			{
				Type:          pb.SetType_KEYLABELOFPOD,
				Name:          "podlabel-pod:a",
				HashedSetName: "azure-npm-3922407721",
				Included:      true,
			},
			{
				Type:          pb.SetType_NAMESPACE,
				Name:          "ns-y",
				HashedSetName: "azure-npm-2837910840",
				Included:      true,
			},
		},
		DstList:       []*pb.RuleResponse_SetInfo{},
		Allowed:       false,
		Direction:     pb.Direction_EGRESS,
		UnsortedIpset: map[string]string{},
		JumpTo:        "AZURE-NPM-EGRESS-2697641196",
		Comment:       "[EGRESS-POLICY-y/base-FROM-podlabel-pod:a-AND-ns-y-IN-ns-y]",
	}}

	testCases := map[string]*test{
		"allowed rule": {input: rawchain, expected: expected},
	}

	c := &Converter{
		EnableV2NPM: true,
	}
	err := c.initConverterFile(npmCacheFileV2)
	if err != nil {
		t.Errorf("error during initilizing converter : %v", err)
	}

	for name, test := range testCases {
		test := test
		t.Run(name, func(t *testing.T) {
			actualResponseArr, err := c.getRulesFromChain(test.input)
			if err != nil {
				t.Errorf("error during get rules : %v", err)
			}
			require.Exactly(t, test.expected, actualResponseArr)
		})
	}
}

func TestGetModulesFromRule(t *testing.T) {
	m0 := &NPMIPtable.Module{
		Verb:           "set",
		OptionValueMap: map[string][]string{"match-set": {"azure-npm-2837910840", "dst"}},
	} // ns-y - NAMESPACE
	m1 := &NPMIPtable.Module{
		Verb:           "tcp",
		OptionValueMap: map[string][]string{"dport": {"8000"}},
	}
	m2 := &NPMIPtable.Module{
		Verb:           "udp",
		OptionValueMap: map[string][]string{"sport": {"53"}},
	}

	s0 := &pb.RuleResponse_SetInfo{
		Type:          pb.SetType_NAMESPACE,
		Name:          "ns-y",
		HashedSetName: "azure-npm-2837910840",
		Included:      true,
	}

	modules := []*NPMIPtable.Module{m0, m1, m2}
	dstList := []*pb.RuleResponse_SetInfo{s0}

	expectedRuleResponse := &pb.RuleResponse{
		Chain:         "TEST",
		SrcList:       []*pb.RuleResponse_SetInfo{},
		DstList:       dstList,
		Protocol:      "",
		DPort:         8000,
		SPort:         53,
		Allowed:       true,
		Direction:     pb.Direction_INGRESS,
		UnsortedIpset: make(map[string]string),
	}

	actualRuleResponse := &pb.RuleResponse{
		Chain:     "TEST",
		Protocol:  "",
		Allowed:   true,
		Direction: pb.Direction_INGRESS,
	}

	c := &Converter{
		EnableV2NPM: true,
	}
	err := c.initConverterFile(npmCacheFileV2)
	if err != nil {
		t.Errorf("error during initilizing converter : %v", err)
	}

	err = c.getModulesFromRule(modules, actualRuleResponse)
	if err != nil {
		t.Errorf("error during getNPMIPtable.ModulesFromRule : %v", err)
	}

	require.Exactly(t, expectedRuleResponse, actualRuleResponse)
}
