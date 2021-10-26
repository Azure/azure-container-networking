package translation

import (
	"io/ioutil"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
)

// TODO(jungukcho)
// 1. will use variables in UTs instead of constant "src",  and "dst" for better managements
// 2. need to walk through inputs of tests to remove redundancy
// - Example - TestPodSelectorIPSets and TestNameSpaceSelectorIPSets (while setType is different)
func TestPortType(t *testing.T) {
	tcp := v1.ProtocolTCP
	port8000 := intstr.FromInt(8000)
	var endPort int32 = 8100
	namedPortStr := "serve-tcp"
	namedPortName := intstr.FromString(namedPortStr)

	tests := []struct {
		name     string
		portRule networkingv1.NetworkPolicyPort
		want     netpolPortType
		wantErr  bool
	}{
		{
			name:     "empty",
			portRule: networkingv1.NetworkPolicyPort{},
			want:     numericPort,
		},
		{
			name: "tcp",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
			},
			want: numericPort,
		},
		{
			name: "port 8000",
			portRule: networkingv1.NetworkPolicyPort{
				Port: &port8000,
			},
			want: numericPort,
		},
		{
			name: "tcp port 8000",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
			},
			want: numericPort,
		},
		{
			name: "tcp port 8000-81000",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
				EndPort:  &endPort,
			},
			want: numericPort,
		},
		{
			name: "serve-tcp",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &namedPortName,
			},
			want: namedPort,
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := translator.portType(tt.portRule)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNumericPortRule(t *testing.T) {
	tcp := v1.ProtocolTCP
	port8000 := intstr.FromInt(8000)
	var endPort int32 = 8100
	tests := []struct {
		name         string
		portRule     networkingv1.NetworkPolicyPort
		want         policies.Ports
		wantProtocol string
	}{
		{
			name:         "empty",
			portRule:     networkingv1.NetworkPolicyPort{},
			want:         policies.Ports{},
			wantProtocol: "TCP",
		},
		{
			name: "tcp",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
			},
			want: policies.Ports{
				Port:    0,
				EndPort: 0,
			},
			wantProtocol: "TCP",
		},
		{
			name: "port 8000",
			portRule: networkingv1.NetworkPolicyPort{
				Port: &port8000,
			},
			want: policies.Ports{
				Port:    8000,
				EndPort: 0,
			},
			wantProtocol: "TCP",
		},
		{
			name: "tcp port 8000",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
			},
			want: policies.Ports{
				Port:    8000,
				EndPort: 0,
			},
			wantProtocol: "TCP",
		},
		{
			name: "tcp port 8000-81000",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
				EndPort:  &endPort,
			},
			want: policies.Ports{
				Port:    8000,
				EndPort: 8100,
			},
			wantProtocol: "TCP",
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			portRule, protocol := translator.numericPortRule(&tt.portRule)
			require.Equal(t, tt.want, portRule)
			require.Equal(t, tt.wantProtocol, protocol)
		})
	}
}

func TestNamedPortRuleInfo(t *testing.T) {
	namedPortStr := "serve-tcp"
	namedPort := intstr.FromString(namedPortStr)
	type namedPortOutput struct {
		translatedIPSet *ipsets.TranslatedIPSet
		protocol        string
	}
	tcp := v1.ProtocolTCP
	tests := []struct {
		name     string
		portRule *networkingv1.NetworkPolicyPort
		want     *namedPortOutput
		wantErr  bool
	}{
		{
			name:     "empty",
			portRule: nil,
			want: &namedPortOutput{
				translatedIPSet: nil, // (TODO): Need to check it
				protocol:        ""},
		},
		{
			name: "serve-tcp",
			portRule: &networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &namedPort,
			},

			want: &namedPortOutput{
				translatedIPSet: &ipsets.TranslatedIPSet{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Members: []string{},
				},
				protocol: "TCP"},
		},
		{
			name: "serve-tcp without protocol field",
			portRule: &networkingv1.NetworkPolicyPort{
				Port: &namedPort,
			},
			want: &namedPortOutput{
				translatedIPSet: &ipsets.TranslatedIPSet{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Members: []string{},
				},
				protocol: "TCP",
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			translatedIPSet, protocol := translator.namedPortRuleInfo(tt.portRule)
			got := &namedPortOutput{
				translatedIPSet: translatedIPSet,
				protocol:        protocol,
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNamedPortRule(t *testing.T) {
	namedPortStr := "serve-tcp"
	namedPort := intstr.FromString(namedPortStr)
	type namedPortRuleOutput struct {
		translatedIPSet *ipsets.TranslatedIPSet
		setInfo         policies.SetInfo
		protocol        string
	}
	tcp := v1.ProtocolTCP
	tests := []struct {
		name     string
		portRule *networkingv1.NetworkPolicyPort
		want     *namedPortRuleOutput
		wantErr  bool
	}{
		{
			name:     "empty",
			portRule: nil,
			want: &namedPortRuleOutput{
				translatedIPSet: nil,
				setInfo:         policies.SetInfo{},
				protocol:        ""},
			wantErr: false,
		},
		{
			name: "serve-tcp",
			portRule: &networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &namedPort,
			},

			want: &namedPortRuleOutput{
				translatedIPSet: &ipsets.TranslatedIPSet{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Members: []string{},
				},
				setInfo: policies.SetInfo{
					IPSet: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Included:  false,
					MatchType: policies.DstDstMatch,
				},
				protocol: "TCP"},
		},
		{
			name: "serve-tcp without protocol field",
			portRule: &networkingv1.NetworkPolicyPort{
				Port: &namedPort,
			},
			want: &namedPortRuleOutput{
				translatedIPSet: &ipsets.TranslatedIPSet{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Members: []string{},
				},
				setInfo: policies.SetInfo{
					IPSet: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Included:  false,
					MatchType: policies.DstDstMatch,
				},
				protocol: "TCP",
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			namedPortIPSet, setInfo, protocol := translator.namedPortRule(tt.portRule)
			got := &namedPortRuleOutput{
				translatedIPSet: namedPortIPSet,
				setInfo:         setInfo,
				protocol:        protocol,
			}
			require.Equal(t, tt.want, got)
		})
	}

}

func TestIPBlockSetName(t *testing.T) {
	tests := []struct {
		name            string
		policyName      string
		namemspace      string
		direction       policies.Direction
		ipBlockSetIndex int
		want            string
	}{
		{
			name:            "default/test",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			want:            "test-in-ns-default-0IN",
		},
		{
			name:            "testns/test",
			policyName:      "test",
			namemspace:      "testns",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			want:            "test-in-ns-testns-0IN",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := ipBlockSetName(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIPBlockIPSet(t *testing.T) {
	tests := []struct {
		name            string
		policyName      string
		namemspace      string
		direction       policies.Direction
		ipBlockSetIndex int
		ipBlockRule     *networkingv1.IPBlock
		translatedIPSet *ipsets.TranslatedIPSet
	}{
		{
			name:            "empty ipblock rule",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule:     nil,
			translatedIPSet: nil,
		},
		{
			name:            "incorrect ipblock rule with only except",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: nil,
		},
		{
			name:            "only cidr",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR: "172.17.0.0/16",
			},
			translatedIPSet: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Members: []string{"172.17.0.0/16"},
			},
		},
		{
			name:            "one cidr and one except",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Members: []string{"172.17.0.0/16", "172.17.1.0/24nomatch"},
			},
		},
		{
			name:            "one cidr and multiple except",
			policyName:      "test-network-policy",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24", "172.17.2.0/24"},
			},
			translatedIPSet: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: "test-network-policy-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Members: []string{"172.17.0.0/16", "172.17.1.0/24nomatch", "172.17.2.0/24nomatch"},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := translator.ipBlockIPSet(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex, tt.ipBlockRule)
			require.Equal(t, tt.translatedIPSet, got)
		})
	}
}

func TestIPBlockRule(t *testing.T) {
	tests := []struct {
		name            string
		policyName      string
		namemspace      string
		direction       policies.Direction
		ipBlockSetIndex int
		ipBlockRule     *networkingv1.IPBlock
		translatedIPSet *ipsets.TranslatedIPSet
		setInfo         policies.SetInfo
	}{
		{
			name:            "empty ipblock rule",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule:     nil,
			translatedIPSet: nil,
			setInfo:         policies.SetInfo{},
		},
		{
			name:            "incorrect ipblock rule with only except",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: nil,
			setInfo:         policies.SetInfo{},
		},
		{
			name:            "only cidr",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR: "172.17.0.0/16",
			},
			translatedIPSet: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Members: []string{"172.17.0.0/16"},
			},
			setInfo: policies.SetInfo{
				IPSet: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Included:  false,
				MatchType: policies.SrcMatch,
			},
		},
		{
			name:            "one cidr and one except",
			policyName:      "test",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Members: []string{"172.17.0.0/16", "172.17.1.0/24nomatch"},
			},
			setInfo: policies.SetInfo{
				IPSet: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Included:  false,
				MatchType: policies.SrcMatch,
			},
		},
		{
			name:            "one cidr and multiple except",
			policyName:      "test-network-policy",
			namemspace:      "default",
			direction:       policies.Ingress,
			ipBlockSetIndex: 0,
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24", "172.17.2.0/24"},
			},
			translatedIPSet: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: "test-network-policy-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Members: []string{"172.17.0.0/16", "172.17.1.0/24nomatch", "172.17.2.0/24nomatch"},
			},
			setInfo: policies.SetInfo{
				IPSet: &ipsets.IPSetMetadata{
					Name: "test-network-policy-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Included:  false,
				MatchType: policies.SrcMatch,
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			translatedIPSet, setInfo := translator.ipBlockRule(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex, tt.ipBlockRule)
			require.Equal(t, tt.translatedIPSet, translatedIPSet)
			require.Equal(t, tt.setInfo, setInfo)
		})
	}
}

func TestTargetPodSelectorInfo(t *testing.T) {
	tests := []struct {
		name                 string
		labelSelector        *metav1.LabelSelector
		ops                  []string
		ipSetForAcl          []string
		ipSetForSingleVal    []string
		ipSetNameForMultiVal map[string][]string
	}{
		{
			name: "all pods match",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			ops:                  []string{""},
			ipSetForAcl:          []string{""},
			ipSetForSingleVal:    []string{""},
			ipSetNameForMultiVal: map[string][]string{},
		},
		{
			name: "only match labels",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			ops:                  []string{""},
			ipSetForAcl:          []string{"label:src"},
			ipSetForSingleVal:    []string{"label:src"},
			ipSetNameForMultiVal: map[string][]string{},
		},
		{
			name: "match labels and match expression with with Exists OP",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			ops:                  []string{"", ""},
			ipSetForAcl:          []string{"label:src", "label"},
			ipSetForSingleVal:    []string{"label:src", "label"},
			ipSetNameForMultiVal: map[string][]string{},
		},
		{
			name: "match labels and match expression with single value and In OP",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			ops:                  []string{"", ""},
			ipSetForAcl:          []string{"label:src", "labelIn:src"},
			ipSetForSingleVal:    []string{"label:src", "labelIn:src"},
			ipSetNameForMultiVal: map[string][]string{},
		},
		{
			name: "match labels and match expression with single value and NotIn OP",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelNotIn",
						Operator: metav1.LabelSelectorOpNotIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			ops:                  []string{"", "!"},
			ipSetForAcl:          []string{"label:src", "labelNotIn:src"},
			ipSetForSingleVal:    []string{"label:src", "labelNotIn:src"},
			ipSetNameForMultiVal: map[string][]string{},
		},
		{
			name: "match labels and match expression with multiple values and In and NotExist",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k0": "v0",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "k1",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"v10",
							"v11",
						},
					},
					{
						Key:      "k2",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   []string{},
					},
				},
			},
			ops:               []string{"", "!", ""},
			ipSetForAcl:       []string{"k0:v0", "k2", "k1:v10:v11"},
			ipSetForSingleVal: []string{"k0:v0", "k2", "k1:v10", "k1:v11"},
			ipSetNameForMultiVal: map[string][]string{
				"k1:v10:v11": {"k1:v10", "k1:v11"},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ops, ipSetForAcl, ipSetForSingleVal, ipSetNameForMultiVal := translator.targetPodSelectorInfo(tt.labelSelector)
			require.Equal(t, tt.ops, ops)
			require.Equal(t, tt.ipSetForAcl, ipSetForAcl)
			require.Equal(t, tt.ipSetForSingleVal, ipSetForSingleVal)
			require.Equal(t, tt.ipSetNameForMultiVal, ipSetNameForMultiVal)
		})
	}
}

func TestAllPodsSelectorInNs(t *testing.T) {
	tests := []struct {
		name              string
		namespace         string
		matchType         policies.MatchType
		podSelectorIPSets []*ipsets.TranslatedIPSet
		podSelectorList   []policies.SetInfo
	}{
		{
			name:      "all pods selector in default namespace in ingress",
			namespace: "default",
			matchType: policies.DstMatch,
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "default",
						Type: ipsets.Namespace,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "default",
						Type: ipsets.Namespace,
					},
					Included:  false,
					MatchType: policies.DstMatch,
				},
			},
		},
		{
			name:      "all pods selector in test namespace in ingress",
			namespace: "test",
			matchType: policies.DstMatch,
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "test",
						Type: ipsets.Namespace,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "test",
						Type: ipsets.Namespace,
					},
					Included:  false,
					MatchType: policies.DstMatch,
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorIPSets, podSelectorList := translator.allPodsSelectorInNs(tt.namespace, tt.matchType)
			require.Equal(t, tt.podSelectorIPSets, podSelectorIPSets)
			require.Equal(t, tt.podSelectorList, podSelectorList)
		})
	}
}

func TestPodSelectorIPSets(t *testing.T) {
	tests := []struct {
		name                 string
		ipSetForSingleVal    []string
		ipSetNameForMultiVal map[string][]string
		podSelectorIPSets    []*ipsets.TranslatedIPSet
	}{
		{
			name:                 "one single value ipset (keyValueLabel)",
			ipSetForSingleVal:    []string{"label:src"},
			ipSetNameForMultiVal: map[string][]string{},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
			},
		},
		{
			name:                 "two single value ipsets (KeyValueLabel and keyLable) ",
			ipSetForSingleVal:    []string{"label:src", "label"},
			ipSetNameForMultiVal: map[string][]string{},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfPod,
					},
					Members: []string{},
				},
			},
		},
		{
			name:                 "two single value ipsets (two KeyValueLabel)",
			ipSetForSingleVal:    []string{"label:src", "labelIn:src"},
			ipSetNameForMultiVal: map[string][]string{},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
			},
		},
		{
			name:              "four single value ipsets and one multiple value ipset (four KeyValueLabel, one KeyLabel, and one nestedKeyValueLabel)",
			ipSetForSingleVal: []string{"k0:v0", "k2", "k1:v10", "k1:v11"},
			ipSetNameForMultiVal: map[string][]string{
				"k1:v10:v11": {"k1:v10", "k1:v11"},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v10",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v11",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v10:v11",
						Type: ipsets.NestedLabelOfPod,
					},
					Members: []string{"k1:v10", "k1:v11"},
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorIPSets := translator.podSelectorIPSets(tt.ipSetForSingleVal, tt.ipSetNameForMultiVal)
			require.Equal(t, tt.podSelectorIPSets, podSelectorIPSets)
		})
	}
}

func TestPodSelectorRule(t *testing.T) {
	matchType := policies.DstMatch
	tests := []struct {
		name            string
		matchType       policies.MatchType
		ops             []string
		ipSetForAcl     []string
		podSelectorList []policies.SetInfo
	}{
		{
			name:        "one ipset of podSelector for acl in ingress",
			matchType:   matchType,
			ops:         []string{""},
			ipSetForAcl: []string{"label:src"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "two ipsets of podSelector (one keyvalue and one only key) for acl in ingress",
			matchType:   policies.DstMatch,
			ops:         []string{"", ""},
			ipSetForAcl: []string{"label:src", "label"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "two ipsets of podSelector (two keyvalue) for acl in ingress",
			matchType:   matchType,
			ops:         []string{"", ""},
			ipSetForAcl: []string{"label:src", "labelIn:src"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: policies.DstMatch,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: policies.DstMatch,
				},
			},
		},
		{
			name:        "two ipsets of podSelector (one included and one non-included ipset) for acl in ingress",
			matchType:   matchType,
			ops:         []string{"", "!"},
			ipSetForAcl: []string{"label:src", "labelNotIn:src"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "three ipsets of podSelector (one included value, one non-included value, and one included netest value) for acl in ingress",
			matchType:   matchType,
			ops:         []string{"", "!", ""},
			ipSetForAcl: []string{"k0:v0", "k2", "k1:v10:v11"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfPod,
					},
					Included:  false,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k1:v10:v11",
						Type: ipsets.NestedLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorList := translator.podSelectorRule(tt.matchType, tt.ops, tt.ipSetForAcl)
			require.Equal(t, tt.podSelectorList, podSelectorList)
		})
	}
}

func TestTargetPodSelector(t *testing.T) {
	matchType := policies.DstMatch
	tests := []struct {
		name              string
		namespace         string
		matchType         policies.MatchType
		labelSelector     *metav1.LabelSelector
		podSelectorIPSets []*ipsets.TranslatedIPSet
		podSelectorList   []policies.SetInfo
	}{
		{
			name:      "all pods selector in default namespace in ingress",
			namespace: "default",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "default",
						Type: ipsets.Namespace,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "default",
						Type: ipsets.Namespace,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "all pods selector in test namespace in ingress",
			namespace: "test",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "test",
						Type: ipsets.Namespace,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "test",
						Type: ipsets.Namespace,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "target pod selector with one label in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "target pod selector with two labels (one keyvalue and one only key) in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfPod,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "target pod selector with two labels (two keyvalue) in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "target pod Selector with two labels (one included and one non-included ipset) for acl in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelNotIn",
						Operator: metav1.LabelSelectorOpNotIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "target pod Selector with three labels (one included value, one non-included value, and one included netest value) for acl in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k0": "v0",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "k1",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"v10",
							"v11",
						},
					},
					{
						Key:      "k2",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   []string{},
					},
				},
			},
			podSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v10",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v11",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v10:v11",
						Type: ipsets.NestedLabelOfPod,
					},
					Members: []string{"k1:v10", "k1:v11"},
				},
			},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfPod,
					},
					Included:  false,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k1:v10:v11",
						Type: ipsets.NestedLabelOfPod,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorIPSets, podSelectorList := translator.targetPodSelector(tt.namespace, tt.matchType, tt.labelSelector)
			require.Equal(t, tt.podSelectorIPSets, podSelectorIPSets)
			require.Equal(t, tt.podSelectorList, podSelectorList)
		})
	}
}

func TestNameSpaceSelectorInfo(t *testing.T) {
	tests := []struct {
		name              string
		labelSelector     *metav1.LabelSelector
		ops               []string
		singleValueLabels []string
	}{
		{
			name: "",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			ops:               []string{""},
			singleValueLabels: []string{""},
		},
		{
			name: "only match labels",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			ops:               []string{""},
			singleValueLabels: []string{"label:src"},
		},
		{
			name: "match labels and match expression with with Exists OP",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			ops:               []string{"", ""},
			singleValueLabels: []string{"label:src", "label"},
		},
		{
			name: "match labels and match expression with single value and In OP",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			ops:               []string{"", ""},
			singleValueLabels: []string{"label:src", "labelIn:src"},
		},
		{
			name: "match labels and match expression with single value and NotIn OP",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelNotIn",
						Operator: metav1.LabelSelectorOpNotIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			ops:               []string{"", "!"},
			singleValueLabels: []string{"label:src", "labelNotIn:src"},
		},
		{
			name: "match labels and match expression with multiple values and In and NotExist",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k0": "v0",
				},
				// Multiple values are ignored in namespace case
				// Refer to FlattenNameSpaceSelector function in parseSelector.go
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "k1",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"v10",
							"v11",
						},
					},
					{
						Key:      "k2",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   []string{},
					},
				},
			},
			ops:               []string{"", "!"},
			singleValueLabels: []string{"k0:v0", "k2"},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ops, singleValueLabels := translator.nameSpaceSelectorInfo(tt.labelSelector)
			require.Equal(t, tt.ops, ops)
			require.Equal(t, tt.singleValueLabels, singleValueLabels)
		})
	}
}

func TestAllNameSpaceRule(t *testing.T) {
	matchType := policies.SrcMatch
	tests := []struct {
		name             string
		matchType        policies.MatchType
		nsSelectorIPSets []*ipsets.TranslatedIPSet
		nsSelectorList   []policies.SetInfo
	}{
		{
			name:      "pods from all namespaces in ingress",
			matchType: matchType,
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.KubeAllNamespacesFlag,
						Type: ipsets.Namespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: util.KubeAllNamespacesFlag,
						Type: ipsets.Namespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets, nsSelectorList := translator.allNameSpaceRule(tt.matchType)
			require.Equal(t, tt.nsSelectorIPSets, nsSelectorIPSets)
			require.Equal(t, tt.nsSelectorList, nsSelectorList)
		})
	}
}

func TestNameSpaceSelectorIPSets(t *testing.T) {
	tests := []struct {
		name              string
		singleValueLabels []string
		nsSelectorIPSets  []*ipsets.TranslatedIPSet
	}{
		{
			name:              "one single value ipset (keyValueLabel)",
			singleValueLabels: []string{"label:src"},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
		},
		{
			name:              "two single value ipsets (KeyValueLabel and keyLable) ",
			singleValueLabels: []string{"label:src", "label"},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Members: []string{},
				},
			},
		},
		{
			name:              "two single value ipsets (two KeyValueLabel)",
			singleValueLabels: []string{"label:src", "labelIn:src"},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
		},
		{
			name:              "four single value ipsets (three KeyValueLabel, and one KeyLabel)",
			singleValueLabels: []string{"k0:v0", "k2", "k1:v10", "k1:v11"},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v10",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k1:v11",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets := translator.nameSpaceSelectorIPSets(tt.singleValueLabels)
			require.Equal(t, tt.nsSelectorIPSets, nsSelectorIPSets)
		})
	}
}

func TestNameSpaceSelectorRule(t *testing.T) {
	matchType := policies.SrcMatch
	tests := []struct {
		name              string
		matchType         policies.MatchType
		ops               []string
		singleValueLabels []string
		nsSelectorList    []policies.SetInfo
	}{
		{
			name:              "one ipset of namespaceSelector for acl in ingress",
			matchType:         matchType,
			ops:               []string{""},
			singleValueLabels: []string{"label:src"},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:              "two ipsets of namespaceSelector (one keyvalue and one only key) for acl in ingress",
			matchType:         matchType,
			ops:               []string{"", ""},
			singleValueLabels: []string{"label:src", "label"},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:              "two ipsets of namespaceSelector (two keyvalue) for acl in ingress",
			matchType:         matchType,
			ops:               []string{"", ""},
			singleValueLabels: []string{"label:src", "labelIn:src"},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:              "two ipsets of namespaceSelector (one included and one non-included ipset) for acl in ingress",
			matchType:         matchType,
			ops:               []string{"", "!"},
			singleValueLabels: []string{"label:src", "labelNotIn:src"},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
		{
			name:              "two ipsets of namespaceSelector (one included keyValue and one non-included key) for acl in ingress",
			matchType:         matchType,
			ops:               []string{"", "!"},
			singleValueLabels: []string{"k0:v0", "k2"},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorList := translator.nameSpaceSelectorRule(tt.matchType, tt.ops, tt.singleValueLabels)
			require.Equal(t, tt.nsSelectorList, nsSelectorList)
		})
	}
}

func TestNameSpaceSelector(t *testing.T) {
	matchType := policies.SrcMatch
	tests := []struct {
		name             string
		matchType        policies.MatchType
		labelSelector    *metav1.LabelSelector
		nsSelectorIPSets []*ipsets.TranslatedIPSet
		nsSelectorList   []policies.SetInfo
	}{
		{
			name:      "namespaceSelector for all namespaces in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.KubeAllNamespacesFlag,
						Type: ipsets.Namespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: util.KubeAllNamespacesFlag,
						Type: ipsets.Namespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "namespaceSelector with one label in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				// TODO(jungukcho): check this one
				MatchLabels: map[string]string{
					"test": "",
				},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "test:",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "test:",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "namespaceSelector with one label in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "namespaceSelector with two labels (one keyvalue and one only key) in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "label",
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "namespaceSelector with two labels (two keyvalue) in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelIn",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "namespaceSelector with two labels (one included and one non-included ipset) for acl in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "labelNotIn",
						Operator: metav1.LabelSelectorOpNotIn,
						Values: []string{
							"src",
						},
					},
				},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "namespaceSelector with two labels (one included value and one non-included value) for acl in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"k0": "v0",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "k1",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							"v10",
							"v11",
						},
					},
					{
						Key:      "k2",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   []string{},
					},
				},
			},
			// Multiple values are ignored in namespace case
			// Refer to FlattenNameSpaceSelector function in parseSelector.go
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Members: []string{},
				},
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  true,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  false,
					MatchType: matchType,
				},
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets, nsSelectorList := translator.nameSpaceSelector(tt.matchType, tt.labelSelector)
			require.Equal(t, tt.nsSelectorIPSets, nsSelectorIPSets)
			require.Equal(t, tt.nsSelectorList, nsSelectorList)
		})
	}
}

// Unit tests with network policy yaml files
func readPolicyYaml(policyYaml string) (*networkingv1.NetworkPolicy, error) {
	decode := scheme.Codecs.UniversalDeserializer().Decode
	b, err := ioutil.ReadFile(policyYaml)
	if err != nil {
		return nil, err
	}
	obj, _, err := decode([]byte(b), nil, nil)
	if err != nil {
		return nil, err
	}
	return obj.(*networkingv1.NetworkPolicy), nil
}

// func TestOnlyNamedPorts(t *testing.T) {
// 	policyFile := "testpolicies/named-port.yaml"
// 	netpol, err := readPolicyYaml(policyFile)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	npPolicy := policies.NPMNetworkPolicy{
// 		Name:              netpol.Name,
// 		NameSpace:         netpol.Namespace,
// 		PodSelectorIPSets: []*ipsets.TranslatedIPSet{},
// 		RuleIPSets:        []*ipsets.TranslatedIPSet{},
// 		ACLs:              []*policies.ACLPolicy{},
// 		RawNP:             &networkingv1.NetworkPolicy{},
// 	}

// 	translator := &translator{}

// 	// ops, labelsForSpec, singleValueLabels, multiValuesLabels := translator.podSelectorIPSets(netpol.Namespace, &netpol.Spec.PodSelector, util.IptablesDstFlag)
// 	ops, labelsForSpec, _, _ := translator.targetPodSelectorInfo(netpol.Namespace, &netpol.Spec.PodSelector)
// 	dstList := translator.podSelectorRule(ops, labelsForSpec)
// 	for i, dst := range dstList {
// 		fmt.Printf("%d %+v\n", i, dst)
// 	}

// 	// #2. Get Port Information
// 	// only port case
// 	for _, rule := range netpol.Spec.Ingress {
// 		for _, port := range rule.Ports {
// 			acl := &policies.ACLPolicy{
// 				PolicyID:  netpol.Name, // redundant
// 				Target:    policies.Allowed,
// 				Direction: policies.Ingress,
// 				DstList:   dstList,
// 			}
// 			_, dstPortIPSet, protocol := translator.namedPortRule(&port)
// 			acl.DstList = append(acl.DstList, dstPortIPSet)
// 			acl.Protocol = policies.Protocol(protocol)
// 			npPolicy.ACLs = append(npPolicy.ACLs, acl)
// 		}
// 	}

// 	for _, acl := range npPolicy.ACLs {
// 		for i, src := range acl.SrcList {
// 			fmt.Printf("src %d %+v\n", i, src)
// 		}

// 		for i, src := range acl.SrcList {
// 			fmt.Printf("dst %d %+v\n", i, src)
// 		}
// 	}
// }

// func TestOnlyPort(t *testing.T) {
// 	policyFile := "testpolicies/only-ports.yaml"
// 	netpol, err := readPolicyYaml(policyFile)

// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	npPolicy := policies.NPMNetworkPolicy{
// 		Name:              netpol.Name,
// 		NameSpace:         netpol.Namespace,
// 		PodSelectorIPSets: []*ipsets.TranslatedIPSet{},
// 		RuleIPSets:        []*ipsets.TranslatedIPSet{},
// 		ACLs:              []*policies.ACLPolicy{},
// 		RawNP:             &networkingv1.NetworkPolicy{},
// 	}

// 	// Just need PodSelectorIPSets Information
// 	// 0     0 MARK       udp  --  *      *       0.0.0.0/0            0.0.0.0/0
// 	// udp dpt:100 match-set azure-npm-784554818 dst match-set azure-npm-1519775445 dst
// 	// /* ALLOW-ALL-UDP-PORT-100-TO-app:server-IN-ns-default */ MARK set 0x2000
// 	translator := &translator{}

// 	// #1. Calculate podIPEntry
// 	// ops, labelsForSpec, singleValueLabels, multiValuesLabels := translator.podSelectorIPSets(netpol.Namespace, &netpol.Spec.PodSelector, util.IptablesDstFlag)
// 	ops, labelsForSpec, _, _ := translator.targetPodSelectorInfo(netpol.Namespace, &netpol.Spec.PodSelector)
// 	dstList := translator.podSelectorRule(ops, labelsForSpec)
// 	for i, dst := range dstList {
// 		fmt.Printf("%d %+v\n", i, dst)
// 	}

// 	// #2. Get Port Information
// 	// only port case
// 	for _, rule := range netpol.Spec.Ingress {
// 		for _, port := range rule.Ports {
// 			acl := &policies.ACLPolicy{
// 				PolicyID:  netpol.Name, // redundant
// 				Target:    policies.Allowed,
// 				Direction: policies.Ingress,
// 				DstList:   dstList,
// 			}
// 			portInfo, protocol := translator.numericPortRule(&port)
// 			acl.DstPorts = portInfo
// 			acl.Protocol = policies.Protocol(protocol)
// 			npPolicy.ACLs = append(npPolicy.ACLs, acl)
// 		}
// 	}

// 	for _, acl := range npPolicy.ACLs {
// 		fmt.Printf("%+v %+v\n", acl.SrcPorts, acl.SrcList)
// 	}
// }

// Just play with network policy and  get hash values
// to understand how iptm.IptEntry is created by using ACLPolicy (will be deleted)
/*
func TestHashValues(t *testing.T) {
	// ns-testnamespace : azure-npm-2173871756
	// k0:v0 : azure-npm-901539978
	// k2 : azure-npm-2503834632
	// k1:v10:v11 : azure-npm-992793578
	// label : azure-npm-4137097213
	// label:src : azure-npm-1570928457
	// app:backend : azure-npm-3038731686
	// role:frontend : azure-npm-2574419033
	// namedport:serve-80 : azure-npm-3050895063
	// ns-netpol-4537-x : azure-npm-3024785582
	// pod:a : azure-npm-3545492025
	// pod:x : azure-npm-3931377262
	// pod:b : azure-npm-3495159168
	// pod:c : azure-npm-3511936787
	// app:test : azure-npm-2817129730
	// app:server : azure-npm-1519775445

	// ns-ns:netpol-4537-x : azure-npm-1052046537
	// ns-ns:netpol-4537-y : azure-npm-1035268918
	// app:test : azure-npm-2817129730
	// app:int : azure-npm-3357534811
	// pod:a:x : azure-npm-4176901587 pod:a : azure-npm-3545492025
	// pod:a:x : azure-npm-4176901587 pod:x : azure-npm-3931377262
	// pod:b:c : azure-npm-3025643863 pod:b : azure-npm-3495159168
	// pod:b:c : azure-npm-3025643863 pod:c : azure-npm-3511936787
	// app:test:int : azure-npm-231489545 app:test : azure-npm-2817129730
	// app:test:int : azure-npm-231489545 app:int : azure-npm-3357534811

	labels := []string{
		"ns-testnamespace",
		"label",
		"label:src",
		"app:backend",
		"role:frontend",
		"namedport:serve-80",
		"ns-netpol-4537-x",
		"pod:a",
		"pod:x",
		"pod:b",
		"pod:c",
		"app:test",
		"app:int",
		"k0:v0",
		"k2",
		"k1:v10:v11",
		"ns-default",
		"app:server",
	}

	for _, label := range labels {
		fmt.Printf("%s : %s\n", label, util.GetHashedName(label))
	}

	expectedLists := map[string][]string{
		"app:test:int": {
			"app:test",
			"app:int",
		},
		"ns-ns:netpol-4537-x": {},
		"ns-ns:netpol-4537-y": {},
		"pod:a:x": {
			"pod:a",
			"pod:x",
		},
		"pod:b:c": {
			"pod:b",
			"pod:c",
		},
	}

	for key, lists := range expectedLists {
		if len(lists) == 0 {
			fmt.Printf("%s : %s  \n", key, util.GetHashedName(key))
		} else {
			for _, list := range lists {
				fmt.Printf("%s : %s %s : %s \n", key, util.GetHashedName(key), list, util.GetHashedName(list))
			}
		}
	}
}

func TestAllowAll(t *testing.T) {
	// set [ns-testnamespace]
	// lists map[]
	// iptEntry &{Command: Name: Chain:AZURE-NPM-INGRESS-DROPS Flag: LockWaitTimeInSeconds:
	// 	Specs:[
	// 		-m set --match-set azure-npm-2173871756 dst
	// 		-j DROP
	// 		-m comment --comment DROP-ALL-TO-ns-testnamespace]}

	// // Does not run getDefaultDropEntries(npNs, npObj.Spec.PodSelector, hasIngress, hasEgress)...) function
	// // allowExternal
	// set [ns-testnamespace]
	// lists map[]
	// iptEntry &{Command: Name: Chain:AZURE-NPM-INGRESS-PORT Flag: LockWaitTimeInSeconds:
	// 	Specs:[
	// 		-m set --match-set azure-npm-2173871756 dst
	// 		-j MARK --set-mark 0x2000
	// 		-m comment --comment ALLOW-ALL-TO-ns-testnamespace]}



	// set [ns-netpol-4537-x pod:a pod:x]

	// lists map[ns-ns:netpol-4537-x:[] ns-ns:netpol-4537-y:[] pod:a:x:[pod:a pod:x]]

	// iptEntry &{Command: Name: Chain:AZURE-NPM-INGRESS-FROM Flag: LockWaitTimeInSeconds:
	// 	Specs:[-m set ! --match-set azure-npm-1052046537 src -m set --match-set azure-npm-3024785582 dst -m set --match-set azure-npm-4176901587 dst -j MARK --set-mark 0x2000 -m comment --comment ALLOW-ns-!ns:netpol-4537-x-TO-pod:a:x-IN-ns-netpol-4537-x]}
	// iptEntry &{Command: Name: Chain:AZURE-NPM-INGRESS-FROM Flag: LockWaitTimeInSeconds:
	// 	Specs:[-m set ! --match-set azure-npm-1035268918 src -m set --match-set azure-npm-3024785582 dst -m set --match-set azure-npm-4176901587 dst -j MARK --set-mark 0x2000 -m comment --comment ALLOW-ns-!ns:netpol-4537-y-TO-pod:a:x-IN-ns-netpol-4537-x]}
	// iptEntry &{Command: Name: Chain:AZURE-NPM-INGRESS-DROPS Flag: LockWaitTimeInSeconds:
	// 	Specs:[-m set --match-set azure-npm-3024785582 dst -m set --match-set azure-npm-4176901587 dst -j DROP -m comment --comment DROP-ALL-TO-pod:a:x-IN-ns-netpol-4537-x]}

	tests := []struct {
		name             string
		netpolPolicyFile string
	}{
		{
			name:             "deny-all",
			netpolPolicyFile: "testpolicies/testing/deny-all.yaml",
		},
		{
			name:             "allow-all",
			netpolPolicyFile: "testpolicies/testing/allow-all.yaml",
		},
		{
			name:             "allow-all-from-all",
			netpolPolicyFile: "testpolicies/testing/allow-all-from-all.yaml",
		},
		{
			name:             "allow-ns-y-z-pod-b-c.yaml",
			netpolPolicyFile: "testpolicies/allow-ns-y-z-pod-b-c.yaml",
		},
		{
			name:             "allow only port",
			netpolPolicyFile: "testpolicies/only-ports.yaml",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			multiPodSlector, err := readPolicyYaml(tt.netpolPolicyFile)
			if err != nil {
				t.Fatal(err)
			}

			util.IsNewNwPolicyVerFlag = true
			sets, _, lists, _, _, iptEntries := translatePolicy(multiPodSlector)
			fmt.Println(tt.name)
			fmt.Printf("set %+v\n", sets)
			fmt.Printf("lists %+v\n", lists)
			for _, iptEntry := range iptEntries {
				fmt.Printf("iptEntry %+v\n", iptEntry)
			}
		})
	}
}


func IptEntrySpecFromOpAndLabel(op, label, srcOrDstFlag string) []string {
	partialSpec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		op,
		util.IptablesMatchSetFlag,
		util.GetHashedName(label),
		srcOrDstFlag,
	}

	return util.DropEmptyFields(partialSpec)
}

func IptEntryForNamedPortRule(ns string, ops, labels []string, srcOrDstFlag string) []string {
	spec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("ns-" + ns),
		srcOrDstFlag,
	}

	for i := range ops {
		// TODO need to change this logic, create a list of lsts here and have a single match against it
		spec = append(spec, IptEntrySpecFromOpAndLabel(ops[i], labels[i], srcOrDstFlag)...)
	}

	return spec
}

func (t *translator) IptEntryForPodSelectorIPSets(ns string, ops, labels []string, srcOrDstFlag string) []string {
	spec := []string{
		util.IptablesModuleFlag,
		util.IptablesSetModuleFlag,
		util.IptablesMatchSetFlag,
		util.GetHashedName("ns-" + ns),
		srcOrDstFlag,
	}

	for i := range ops {
		// TODO need to change this logic, create a list of lsts here and have a single match against it
		spec = append(spec, IptEntrySpecFromOpAndLabel(ops[i], labels[i], srcOrDstFlag)...)
	}

	return spec
}
*/
// to check how to generate iptableEntry
// func TestSimpleIngress(t *testing.T) {
// 	//policyFile := "testpolicies/deny-all-policy.yaml"
// 	policyFile := "testpolicies/simple-ingress.yaml"
// 	policy, err := readPolicyYaml(policyFile)

// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	translator := &translator{}
// 	//sets, namedPort, lists, ingressIPCidrs, egressIPCidrs, iptEntries := translator.translatePolicy(policy)
// 	sets, _, lists, _, _, iptEntries := translator.translatePolicy(policy)
// 	fmt.Printf("set %+v\n", sets)
// 	fmt.Printf("lists %+v\n", lists)
// 	for _, iptEntry := range iptEntries {
// 		fmt.Printf("iptEntry %+v\n", iptEntry)
// 	}

// 	fmt.Println("******************************")
// 	// TODO(junguk): where should I create this in translation logic?
// 	podIPset := &ipsets.TranslatedIPSet{
// 		Metadata: &ipsets.IPSetMetadata{
// 			Name: "app:backend",
// 			Type: ipsets.KeyValueLabelOfPod,
// 		},
// 		Members: []string{},
// 	}

// 	setInfo := policies.SetInfo{
// 		IPSet:     podIPset.Metadata,
// 		MatchType: util.IptablesDstFlag,
// 	}

// 	srcList := []policies.SetInfo{setInfo}

// 	nsIPset := &ipsets.TranslatedIPSet{
// 		Metadata: &ipsets.IPSetMetadata{
// 			Name: "ns-testnamespace",
// 			Type: ipsets.KeyLabelOfNameSpace,
// 		},
// 		Members: []string{},
// 	}

// 	setInfo = policies.SetInfo{
// 		IPSet:     nsIPset.Metadata,
// 		MatchType: util.IptablesDstFlag,
// 	}

// 	srcList = append(srcList, setInfo)
// 	// TODO(junguk): how to generate iptables with aclPolicy
// 	// Check how current network policies does..
// 	/*
// 		iptEntry &{Command: Name: Chain:AZURE-NPM-INGRESS-DROPS Flag: LockWaitTimeInSeconds:
// 			Specs:[
// 				-m set --match-set azure-npm-2173871756 dst
// 				-m set --match-set azure-npm-3038731686 dst
// 				-j DROP
// 				-m comment --comment DROP-ALL-TO-app:backend-IN-ns-testnamespace
// 		]}
// 	*/

// 	// TODO(junguk): need to know chain type - "FROM", "PORT", "DROP"
// 	// Target
// 	// Direction
// 	aclPolicy := policies.ACLPolicy{
// 		PolicyID:  policy.Name,
// 		SrcList:   srcList,
// 		DstList:   []policies.SetInfo{},
// 		Target:    policies.Dropped,
// 		Direction: policies.Ingress,
// 	}

// 	// This is how to make tests
// 	chain := ""
// 	if aclPolicy.Target == policies.Dropped {
// 		chain = util.IptablesAzureIngressDropsChain
// 	}

// 	entry := &iptm.IptEntry{
// 		Chain: chain,
// 		Specs: []string{},
// 	}

// 	for _, src := range aclPolicy.SrcList {
// 		entry.Specs = append(
// 			entry.Specs,
// 			util.IptablesModuleFlag,
// 			util.IptablesSetModuleFlag,
// 			util.IptablesMatchSetFlag,
// 			util.GetHashedName(src.IPSet.Name),
// 			src.MatchType,
// 		)
// 	}
// 	if aclPolicy.Target == policies.Dropped {
// 		entry.Specs = append(
// 			entry.Specs,
// 			util.IptablesJumpFlag,
// 			util.IptablesDrop,
// 			util.IptablesModuleFlag,
// 			util.IptablesCommentModuleFlag,
// 			util.IptablesCommentFlag,
// 		)
// 	}
// 	fmt.Printf("%+v\n", entry)
// }
