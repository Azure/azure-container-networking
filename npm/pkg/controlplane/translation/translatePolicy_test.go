package translation

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	nonIncluded  bool   = false
	namedPortStr string = "serve-tcp"
)

// TODO(jungukcho)
// 1. will use variables in UTs instead of constant "src",  and "dst" for better managements
// 2. need to walk through inputs of tests to remove redundancy
// - Example - TestPodSelectorIPSets and TestNameSpaceSelectorIPSets (while setType is different)
func TestPortType(t *testing.T) {
	tcp := v1.ProtocolTCP
	port8000 := intstr.FromInt(8000)
	var endPort int32 = 8100
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
			want:     numericPortType,
		},
		{
			name: "tcp",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
			},
			want: numericPortType,
		},
		{
			name: "port 8000",
			portRule: networkingv1.NetworkPolicyPort{
				Port: &port8000,
			},
			want: numericPortType,
		},
		{
			name: "tcp port 8000",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
			},
			want: numericPortType,
		},
		{
			name: "tcp port 8000-81000",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
				EndPort:  &endPort,
			},
			want: numericPortType,
		},
		{
			name: "serve-tcp",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &namedPortName,
			},
			want: namedPortType,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := portType(tt.portRule)
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			portRule, protocol := numericPortRule(&tt.portRule)
			require.Equal(t, tt.want, portRule)
			require.Equal(t, tt.wantProtocol, protocol)
		})
	}
}

func TestNamedPortRuleInfo(t *testing.T) {
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
				protocol:        "",
			},
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
				protocol: "TCP",
			},
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			translatedIPSet, protocol := namedPortRuleInfo(tt.portRule)
			got := &namedPortOutput{
				translatedIPSet: translatedIPSet,
				protocol:        protocol,
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNamedPortRule(t *testing.T) {
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
				protocol:        "",
			},
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
					Included:  included,
					MatchType: policies.DstDstMatch,
				},
				protocol: "TCP",
			},
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
					Included:  included,
					MatchType: policies.DstDstMatch,
				},
				protocol: "TCP",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			namedPortIPSet, setInfo, protocol := namedPortRule(tt.portRule)
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := ipBlockIPSet(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex, tt.ipBlockRule)
			require.Equal(t, tt.translatedIPSet, got)
		})
	}
}

func TestIPBlockRule(t *testing.T) {
	matchType := policies.SrcMatch
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
				Included:  included,
				MatchType: matchType,
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
				Included:  included,
				MatchType: matchType,
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
				Included:  included,
				MatchType: matchType,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			translatedIPSet, setInfo := ipBlockRule(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex, tt.ipBlockRule)
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
		ipSetForACL          []string
		ipSetForSingleVal    []string
		ipSetNameForMultiVal map[string][]string
	}{
		{
			name: "all pods match",
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			ops:                  []string{""},
			ipSetForACL:          []string{""},
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
			ipSetForACL:          []string{"label:src"},
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
			ipSetForACL:          []string{"label:src", "label"},
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
			ipSetForACL:          []string{"label:src", "labelIn:src"},
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
			ipSetForACL:          []string{"label:src", "labelNotIn:src"},
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
			ipSetForACL:       []string{"k0:v0", "k2", "k1:v10:v11"},
			ipSetForSingleVal: []string{"k0:v0", "k2", "k1:v10", "k1:v11"},
			ipSetNameForMultiVal: map[string][]string{
				"k1:v10:v11": {"k1:v10", "k1:v11"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ops, ipSetForACL, ipSetForSingleVal, ipSetNameForMultiVal := targetPodSelectorInfo(tt.labelSelector)
			require.Equal(t, tt.ops, ops)
			require.Equal(t, tt.ipSetForACL, ipSetForACL)
			require.Equal(t, tt.ipSetForSingleVal, ipSetForSingleVal)
			require.Equal(t, tt.ipSetNameForMultiVal, ipSetNameForMultiVal)
		})
	}
}

func TestAllPodsSelectorInNs(t *testing.T) {
	matchType := policies.DstMatch
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
			matchType: matchType,
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
					Included:  included,
					MatchType: matchType,
				},
			},
		},
		{
			name:      "all pods selector in test namespace in ingress",
			namespace: "test",
			matchType: matchType,
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
					Included:  included,
					MatchType: matchType,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorIPSets, podSelectorList := allPodsSelectorInNs(tt.namespace, tt.matchType)
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorIPSets := podSelectorIPSets(tt.ipSetForSingleVal, tt.ipSetNameForMultiVal)
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
		ipSetForACL     []string
		podSelectorList []policies.SetInfo
	}{
		{
			name:        "one ipset of podSelector for acl in ingress",
			matchType:   matchType,
			ops:         []string{""},
			ipSetForACL: []string{"label:src"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "two ipsets of podSelector (one keyvalue and one only key) for acl in ingress",
			matchType:   policies.DstMatch,
			ops:         []string{"", ""},
			ipSetForACL: []string{"label:src", "label"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "two ipsets of podSelector (two keyvalue) for acl in ingress",
			matchType:   matchType,
			ops:         []string{"", ""},
			ipSetForACL: []string{"label:src", "labelIn:src"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "two ipsets of podSelector (one included and one non-included ipset) for acl in ingress",
			matchType:   matchType,
			ops:         []string{"", "!"},
			ipSetForACL: []string{"label:src", "labelNotIn:src"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  nonIncluded,
					MatchType: matchType,
				},
			},
		},
		{
			name:        "three ipsets of podSelector (one included value, one non-included value, and one included netest value) for acl in ingress",
			matchType:   matchType,
			ops:         []string{"", "!", ""},
			ipSetForACL: []string{"k0:v0", "k2", "k1:v10:v11"},
			podSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k0:v0",
						Type: ipsets.KeyValueLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfPod,
					},
					Included:  nonIncluded,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k1:v10:v11",
						Type: ipsets.NestedLabelOfPod,
					},
					Included:  included,
					MatchType: matchType,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podSelectorList := podSelectorRule(tt.matchType, tt.ops, tt.ipSetForACL)
			require.Equal(t, tt.podSelectorList, podSelectorList)
		})
	}
}

func TestTargetPodSelector(t *testing.T) {
	matchType := policies.DstMatch
	var nilSlices []string
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
				ipsets.NewTranslatedIPSet("default", ipsets.Namespace, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("default", ipsets.Namespace, included, matchType),
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
				ipsets.NewTranslatedIPSet("test", ipsets.Namespace, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("test", ipsets.Namespace, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
				ipsets.NewTranslatedIPSet("label", ipsets.KeyLabelOfPod, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, matchType),
				policies.NewSetInfo("label", ipsets.KeyLabelOfPod, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
				ipsets.NewTranslatedIPSet("labelIn:src", ipsets.KeyValueLabelOfPod, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, matchType),
				policies.NewSetInfo("labelIn:src", ipsets.KeyValueLabelOfPod, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
				ipsets.NewTranslatedIPSet("labelNotIn:src", ipsets.KeyValueLabelOfPod, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, matchType),
				policies.NewSetInfo("labelNotIn:src", ipsets.KeyValueLabelOfPod, nonIncluded, matchType),
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
				ipsets.NewTranslatedIPSet("k0:v0", ipsets.KeyValueLabelOfPod, nilSlices),
				ipsets.NewTranslatedIPSet("k1:v10:v11", ipsets.NestedLabelOfPod, []string{"k1:v10", "k1:v11"}),
				ipsets.NewTranslatedIPSet("k1:v10", ipsets.KeyValueLabelOfPod, nilSlices),
				ipsets.NewTranslatedIPSet("k1:v11", ipsets.KeyValueLabelOfPod, nilSlices),
				ipsets.NewTranslatedIPSet("k2", ipsets.KeyLabelOfPod, nilSlices),
			},
			podSelectorList: []policies.SetInfo{
				policies.NewSetInfo("k0:v0", ipsets.KeyValueLabelOfPod, included, matchType),
				policies.NewSetInfo("k1:v10:v11", ipsets.NestedLabelOfPod, included, matchType),
				policies.NewSetInfo("k2", ipsets.KeyLabelOfPod, nonIncluded, matchType),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var podSelectorIPSets []*ipsets.TranslatedIPSet
			var podSelectorList []policies.SetInfo
			if tt.namespace == "" {
				podSelectorIPSets, podSelectorList = podSelector(tt.matchType, tt.labelSelector)
			} else {
				podSelectorIPSets, podSelectorList = podSelectorWithNS(tt.namespace, tt.matchType, tt.labelSelector)
			}
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ops, singleValueLabels := nameSpaceSelectorInfo(tt.labelSelector)
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
					Included:  included,
					MatchType: matchType,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets, nsSelectorList := allNameSpaceRule(tt.matchType)
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

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets := nameSpaceSelectorIPSets(tt.singleValueLabels)
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
					Included:  included,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  included,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  included,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  nonIncluded,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  nonIncluded,
					MatchType: matchType,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorList := nameSpaceSelectorRule(tt.matchType, tt.ops, tt.singleValueLabels)
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
						Type: ipsets.KeyLabelOfNamespace,
					},
					Members: []string{},
				},
			},
			nsSelectorList: []policies.SetInfo{
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: util.KubeAllNamespacesFlag,
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  included,
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
					Included:  included,
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
					Included:  included,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "label",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  included,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  included,
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "labelNotIn:src",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  nonIncluded,
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
						},
					},
					{
						Key:      "k2",
						Operator: metav1.LabelSelectorOpDoesNotExist,
						Values:   []string{},
					},
				},
			},
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
						Name: "k1:v10",
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
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k1:v10",
						Type: ipsets.KeyValueLabelOfNamespace,
					},
					Included:  included,
					MatchType: matchType,
				},
				{
					IPSet: &ipsets.IPSetMetadata{
						Name: "k2",
						Type: ipsets.KeyLabelOfNamespace,
					},
					Included:  nonIncluded,
					MatchType: matchType,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets, nsSelectorList := nameSpaceSelector(tt.matchType, tt.labelSelector)
			require.Equal(t, tt.nsSelectorIPSets, nsSelectorIPSets)
			require.Equal(t, tt.nsSelectorList, nsSelectorList)
		})
	}
}

func TestAllowAllTraffic(t *testing.T) {
	matchType := policies.SrcMatch
	tests := []struct {
		name             string
		matchType        policies.MatchType
		nsSelectorIPSets *ipsets.TranslatedIPSet
		nsSelectorList   policies.SetInfo
	}{
		{
			name:      "Allow all traffic from all namespaces in ingress",
			matchType: matchType,
			nsSelectorIPSets: &ipsets.TranslatedIPSet{
				Metadata: &ipsets.IPSetMetadata{
					Name: util.KubeAllNamespacesFlag,
					Type: ipsets.Namespace,
				},
				Members: []string{},
			},
			nsSelectorList: policies.SetInfo{
				IPSet: &ipsets.IPSetMetadata{
					Name: util.KubeAllNamespacesFlag,
					Type: ipsets.Namespace,
				},
				Included:  included,
				MatchType: matchType,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			nsSelectorIPSets, nsSelectorList := allowAllTraffic(tt.matchType)
			require.Equal(t, tt.nsSelectorIPSets, nsSelectorIPSets)
			require.Equal(t, tt.nsSelectorList, nsSelectorList)
		})
	}
}

func TestDefaultDropACL(t *testing.T) {
	direction := policies.Ingress
	tests := []struct {
		name       string
		policyName string
		policyNS   string
		direction  policies.Direction
		dropACL    *policies.ACLPolicy
	}{
		{
			name:       "Default drop acl for default/test",
			policyName: "test",
			policyNS:   "default",
			direction:  direction,
			dropACL: &policies.ACLPolicy{
				PolicyID:  "azure-acl-default-test",
				Target:    policies.Dropped,
				Direction: direction,
			},
		},
		{
			name:       "Default drop acl for testns/test",
			policyName: "test",
			policyNS:   "testns",
			direction:  direction,
			dropACL: &policies.ACLPolicy{
				PolicyID:  "azure-acl-testns-test",
				Target:    policies.Dropped,
				Direction: direction,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			dropACL := defaultDropACL(tt.policyNS, tt.policyName, tt.direction)
			require.Equal(t, tt.dropACL, dropACL)
		})
	}
}

func TestPortRuleWithNamedPort(t *testing.T) {
	namedPort := intstr.FromString(namedPortStr)
	tcp := v1.ProtocolTCP
	matchType := policies.DstDstMatch
	tests := []struct {
		name       string
		portRule   *networkingv1.NetworkPolicyPort
		ruleIPSets []*ipsets.TranslatedIPSet
		acl        *policies.ACLPolicy
	}{
		{
			name: "serve-tcp",
			portRule: &networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &namedPort,
			},
			ruleIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Members: []string{},
				},
			},
			acl: &policies.ACLPolicy{
				DstList: []policies.SetInfo{
					{
						IPSet: &ipsets.IPSetMetadata{
							Name: util.NamedPortIPSetPrefix + "serve-tcp",
							Type: ipsets.NamedPorts,
						},
						Included:  included,
						MatchType: matchType,
					},
				},
				Protocol: "TCP",
			},
		},
		{
			name: "serve-tcp without protocol field",
			portRule: &networkingv1.NetworkPolicyPort{
				Port: &namedPort,
			},
			ruleIPSets: []*ipsets.TranslatedIPSet{
				{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
					Members: []string{},
				},
			},
			acl: &policies.ACLPolicy{
				DstList: []policies.SetInfo{
					{
						IPSet: &ipsets.IPSetMetadata{
							Name: util.NamedPortIPSetPrefix + "serve-tcp",
							Type: ipsets.NamedPorts,
						},
						Included:  included,
						MatchType: matchType,
					},
				},
				Protocol: "TCP",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ruleIPSets := []*ipsets.TranslatedIPSet{}
			acl := &policies.ACLPolicy{}
			ruleIPSets = portRule(ruleIPSets, acl, tt.portRule, namedPortType)
			require.Equal(t, tt.ruleIPSets, ruleIPSets)
			require.Equal(t, tt.acl, acl)
		})
	}
}

func TestPortRuleWithNumericPort(t *testing.T) {
	tcp := v1.ProtocolTCP
	port8000 := intstr.FromInt(8000)
	var endPort int32 = 8100
	tests := []struct {
		name     string
		portRule *networkingv1.NetworkPolicyPort
		acl      *policies.ACLPolicy
	}{
		{
			name: "tcp",
			portRule: &networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
			},
			acl: &policies.ACLPolicy{
				DstPorts: policies.Ports{
					Port:    0,
					EndPort: 0,
				},
				Protocol: "TCP",
			},
		},
		{
			name: "port 8000",
			portRule: &networkingv1.NetworkPolicyPort{
				Port: &port8000,
			},
			acl: &policies.ACLPolicy{
				DstPorts: policies.Ports{
					Port:    8000,
					EndPort: 0,
				},
				Protocol: "TCP",
			},
		},
		{
			name: "tcp port 8000",
			portRule: &networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
			},
			acl: &policies.ACLPolicy{
				DstPorts: policies.Ports{
					Port:    8000,
					EndPort: 0,
				},
				Protocol: "TCP",
			},
		},
		{
			name: "tcp port 8000-81000",
			portRule: &networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &port8000,
				EndPort:  &endPort,
			},
			acl: &policies.ACLPolicy{
				DstPorts: policies.Ports{
					Port:    8000,
					EndPort: 8100,
				},
				Protocol: "TCP",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			acl := &policies.ACLPolicy{}
			_ = portRule([]*ipsets.TranslatedIPSet{}, acl, tt.portRule, numericPortType)
			require.Equal(t, tt.acl, acl)
		})
	}
}

func TestPeerAndPortRule(t *testing.T) {
	namedPort := intstr.FromString(namedPortStr)
	port8000 := intstr.FromInt(8000)
	var endPort int32 = 8100
	tcp := v1.ProtocolTCP
	matchType := policies.SrcMatch

	setInfos := [][]policies.SetInfo{
		{
			{},
		},
		{
			{},
		},
		{
			{
				IPSet: &ipsets.IPSetMetadata{
					Name: "test-in-ns-default-0IN",
					Type: ipsets.CIDRBlocks,
				},
				Included:  included,
				MatchType: matchType,
			},
		},
		{
			{
				IPSet: &ipsets.IPSetMetadata{
					Name: "label:src",
					Type: ipsets.KeyValueLabelOfNamespace,
				},
				Included:  included,
				MatchType: matchType,
			},
			{
				IPSet: &ipsets.IPSetMetadata{
					Name: "label",
					Type: ipsets.KeyLabelOfNamespace,
				},
				Included:  included,
				MatchType: matchType,
			},
		},
		{
			{
				IPSet: &ipsets.IPSetMetadata{
					Name: "k0:v0",
					Type: ipsets.KeyValueLabelOfPod,
				},
				Included:  included,
				MatchType: matchType,
			},
			{
				IPSet: &ipsets.IPSetMetadata{
					Name: "k2",
					Type: ipsets.KeyLabelOfPod,
				},
				Included:  nonIncluded,
				MatchType: matchType,
			},
			{
				IPSet: &ipsets.IPSetMetadata{
					Name: "k1:v10:v11",
					Type: ipsets.NestedLabelOfPod,
				},
				Included:  included,
				MatchType: matchType,
			},
		},
	}

	// TODO(jungukcho): add test case with multiple ports
	tests := []struct {
		name      string
		ports     []networkingv1.NetworkPolicyPort
		npmNetPol *policies.NPMNetworkPolicy
	}{
		{
			name: "tcp port 8000-81000",
			ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &tcp,
					Port:     &port8000,
					EndPort:  &endPort,
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      namedPortStr,
				NameSpace: "default",
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstPorts: policies.Ports{
							Port:    8000,
							EndPort: 8100,
						},
						Protocol: "TCP",
					},
				},
			},
		},
		{
			name: "serve-tcp",
			ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &tcp,
					Port:     &namedPort,
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      namedPortStr,
				NameSpace: "default",
				RuleIPSets: []*ipsets.TranslatedIPSet{
					{
						Metadata: &ipsets.IPSetMetadata{
							Name: util.NamedPortIPSetPrefix + "serve-tcp",
							Type: ipsets.NamedPorts,
						},
						Members: []string{},
					},
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstList: []policies.SetInfo{
							{
								IPSet: &ipsets.IPSetMetadata{
									Name: util.NamedPortIPSetPrefix + "serve-tcp",
									Type: ipsets.NamedPorts,
								},
								Included:  included,
								MatchType: policies.DstDstMatch,
							},
						},
						Protocol: "TCP",
					},
				},
			},
		},
		{
			name: "serve-tcp with ipBlock SetInfo",
			ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &tcp,
					Port:     &namedPort,
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      namedPortStr,
				NameSpace: "default",
				RuleIPSets: []*ipsets.TranslatedIPSet{
					{
						Metadata: &ipsets.IPSetMetadata{
							Name: util.NamedPortIPSetPrefix + "serve-tcp",
							Type: ipsets.NamedPorts,
						},
						Members: []string{},
					},
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList: []policies.SetInfo{
							{
								IPSet: &ipsets.IPSetMetadata{
									Name: "test-in-ns-default-0IN",
									Type: ipsets.CIDRBlocks,
								},
								Included:  included,
								MatchType: matchType,
							},
						},
						DstList: []policies.SetInfo{
							{
								IPSet: &ipsets.IPSetMetadata{
									Name: util.NamedPortIPSetPrefix + "serve-tcp",
									Type: ipsets.NamedPorts,
								},
								Included:  included,
								MatchType: policies.DstDstMatch,
							},
						},
						Protocol: "TCP",
					},
				},
			},
		},
		{
			name: "serve-tcp with namespaceSelector SetInfo",
			ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &tcp,
					Port:     &namedPort,
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      namedPortStr,
				NameSpace: "default",
				RuleIPSets: []*ipsets.TranslatedIPSet{
					{
						Metadata: &ipsets.IPSetMetadata{
							Name: util.NamedPortIPSetPrefix + "serve-tcp",
							Type: ipsets.NamedPorts,
						},
						Members: []string{},
					},
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstList: []policies.SetInfo{
							{
								IPSet: &ipsets.IPSetMetadata{
									Name: util.NamedPortIPSetPrefix + "serve-tcp",
									Type: ipsets.NamedPorts,
								},
								Included:  included,
								MatchType: policies.DstDstMatch,
							},
						},
						Protocol: "TCP",
					},
				},
			},
		},
		{
			name: "serve-tcp with podSelector SetInfo",
			ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &tcp,
					Port:     &namedPort,
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      namedPortStr,
				NameSpace: "default",
				RuleIPSets: []*ipsets.TranslatedIPSet{
					{
						Metadata: &ipsets.IPSetMetadata{
							Name: util.NamedPortIPSetPrefix + "serve-tcp",
							Type: ipsets.NamedPorts,
						},
						Members: []string{},
					},
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstList: []policies.SetInfo{
							{
								IPSet: &ipsets.IPSetMetadata{
									Name: util.NamedPortIPSetPrefix + "serve-tcp",
									Type: ipsets.NamedPorts,
								},
								Included:  included,
								MatchType: policies.DstDstMatch,
							},
						},
						Protocol: "TCP",
					},
				},
			},
		},
	}

	for i, tt := range tests {
		tt := tt
		setInfo := setInfos[i]
		t.Run(tt.name, func(t *testing.T) {
			for _, acl := range tt.npmNetPol.ACLs {
				acl.SrcList = setInfo
			}
			npmNetPol := &policies.NPMNetworkPolicy{
				Name:      tt.npmNetPol.Name,
				NameSpace: tt.npmNetPol.NameSpace,
			}
			peerAndPortRule(npmNetPol, tt.ports, setInfo)
			require.Equal(t, tt.npmNetPol, npmNetPol)
		})
	}
}

func TestTranslateIngress(t *testing.T) {
	tcp := v1.ProtocolTCP
	targetPodMatchType := policies.DstMatch
	peerMatchType := policies.SrcMatch
	// TODO(jungukcho): this nilSlices will be removed.
	var nilSlices []string
	// TODO(jungukcho): add test cases with more complex rules
	tests := []struct {
		name           string
		targetSelector *metav1.LabelSelector
		rules          []networkingv1.NetworkPolicyIngressRule
		npmNetPol      *policies.NPMNetworkPolicy
	}{
		{
			name: "only port in ingress rules",
			targetSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			rules: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &tcp,
						},
					},
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      "serve-tcp",
				NameSpace: "default",
				PodSelectorIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace, nilSlices),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						DstPorts: policies.Ports{
							Port:    0,
							EndPort: 0,
						},
						Protocol: "TCP",
					},
				},
			},
		},
		{
			name: "only ipBlock in ingress rules",
			targetSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			rules: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR:   "172.17.0.0/16",
								Except: []string{"172.17.1.0/24"},
							},
						},
					},
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      "only-ipblock",
				NameSpace: "default",
				PodSelectorIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace, nilSlices),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				RuleIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("only-ipblock-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24nomatch"}),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-only-ipblock",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList: []policies.SetInfo{
							policies.NewSetInfo("only-ipblock-in-ns-default-0IN", ipsets.CIDRBlocks, included, peerMatchType),
						},
					},
				},
			},
		},
		{
			name: "only peer podSelector in ingress rules",
			targetSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			rules: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"peer-podselector-kay": "peer-podselector-value",
								},
							},
						},
					},
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      "only-peer-podSelector",
				NameSpace: "default",
				PodSelectorIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace, nilSlices),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				RuleIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("peer-podselector-kay:peer-podselector-value", ipsets.KeyValueLabelOfPod, nilSlices),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace, nilSlices),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-only-peer-podSelector",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList: []policies.SetInfo{
							policies.NewSetInfo("peer-podselector-kay:peer-podselector-value", ipsets.KeyValueLabelOfPod, included, peerMatchType),
							policies.NewSetInfo("default", ipsets.Namespace, included, peerMatchType),
						},
					},
				},
			},
		},
		{
			name: "only peer nameSpaceSelector in ingress rules",
			targetSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
			rules: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"peer-nsselector-kay": "peer-nsselector-value",
								},
							},
						},
					},
				},
			},
			npmNetPol: &policies.NPMNetworkPolicy{
				Name:      "only-peer-nsSelector",
				NameSpace: "default",
				PodSelectorIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod, nilSlices),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace, nilSlices),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				RuleIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("peer-nsselector-kay:peer-nsselector-value", ipsets.KeyValueLabelOfNamespace, []string{}),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-only-peer-nsSelector",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList: []policies.SetInfo{
							policies.NewSetInfo("peer-nsselector-kay:peer-nsselector-value", ipsets.KeyValueLabelOfNamespace, included, peerMatchType),
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			npmNetPol := &policies.NPMNetworkPolicy{
				Name:      tt.npmNetPol.Name,
				NameSpace: tt.npmNetPol.NameSpace,
			}
			translateIngress(npmNetPol, tt.targetSelector, tt.rules)
			require.Equal(t, tt.npmNetPol, npmNetPol)
		})
	}
}
