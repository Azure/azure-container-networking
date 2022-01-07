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
			t.Parallel()
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
			t.Parallel()
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
				translatedIPSet: nil,
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
				translatedIPSet: ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				protocol:        "TCP",
			},
		},
		{
			name: "serve-tcp without protocol field",
			portRule: &networkingv1.NetworkPolicyPort{
				Port: &namedPort,
			},
			want: &namedPortOutput{
				translatedIPSet: ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				protocol:        "TCP",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
				translatedIPSet: ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				setInfo:         policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, policies.DstDstMatch),
				protocol:        "TCP",
			},
		},
		{
			name: "serve-tcp without protocol field",
			portRule: &networkingv1.NetworkPolicyPort{
				Port: &namedPort,
			},
			want: &namedPortRuleOutput{
				translatedIPSet: ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				setInfo:         policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, policies.DstDstMatch),
				protocol:        "TCP",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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

type ipBlockInfo struct {
	policyName      string
	namemspace      string
	direction       policies.Direction
	matchType       policies.MatchType
	ipBlockSetIndex int
}

func createIPBlockInfo(policyName, ns string, direction policies.Direction, matchType policies.MatchType, ipBlockSetIndex int) *ipBlockInfo {
	return &ipBlockInfo{
		policyName:      policyName,
		namemspace:      ns,
		direction:       direction,
		matchType:       matchType,
		ipBlockSetIndex: ipBlockSetIndex,
	}
}

func TestIPBlockSetName(t *testing.T) {
	tests := []struct {
		name string
		*ipBlockInfo
		want string
	}{
		{
			name:        "default/test (ingress)",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			want:        "test-in-ns-default-0IN",
		},
		{
			name:        "default/test (ingress)",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 1),
			want:        "test-in-ns-default-1IN",
		},
		{
			name:        "testns/test (egress)",
			ipBlockInfo: createIPBlockInfo("test", "testns", policies.Egress, policies.DstMatch, 0),
			want:        "test-in-ns-testns-0OUT",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ipBlockSetName(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex)
			require.Equal(t, tt.want, got)
		})
	}
}

// Specific testsets for 0.0.0.0/0 cidr since it has special handling in the code due to limitation of ipset on linux.
func TestIPBlockIPSet(t *testing.T) {
	tests := []struct {
		name string
		*ipBlockInfo
		ipBlockRule     *networkingv1.IPBlock
		translatedIPSet *ipsets.TranslatedIPSet
	}{
		{
			name:            "empty ipblock rule",
			ipBlockInfo:     createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule:     nil,
			translatedIPSet: nil,
		},
		{
			name:        "incorrect ipblock rule with only except",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: nil,
		},
		{
			name:        "only cidr",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR: "172.17.0.0/16",
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16"}...),
		},
		{
			name:        "one cidr and one element in except",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24 nomatch"}...),
		},
		{
			name:        "one cidr and multiple elements in except",
			ipBlockInfo: createIPBlockInfo("test-network-policy", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24", "172.17.2.0/24"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-network-policy-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24 nomatch", "172.17.2.0/24 nomatch"}...),
		},
		{
			name:        "one cidr and multiple and duplicated elements in except",
			ipBlockInfo: createIPBlockInfo("test-network-policy", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24", "172.17.2.0/24", "172.17.2.0/24"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-network-policy-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24 nomatch", "172.17.2.0/24 nomatch"}...),
		},
		{
			name:        "cidr : 0.0.0.0/0",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR: "0.0.0.0/0",
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"0.0.0.0/1", "128.0.0.0/1"}...),
		},
		{
			name:        "cidr: 0.0.0.0/0 and except: 10.0.0.0/1",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "0.0.0.0/0",
				Except: []string{"10.0.0.0/1"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"0.0.0.0/1", "128.0.0.0/1", "10.0.0.0/1 nomatch"}...),
		},
		{
			name:        "cidr: 0.0.0.0/0 and except: 0.0.0.0/1",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "0.0.0.0/0",
				Except: []string{"0.0.0.0/1"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"0.0.0.0/1 nomatch", "128.0.0.0/1"}...),
		},
		{
			name:        "cidr: 0.0.0.0/0 and except: 128.0.0.0/1",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "0.0.0.0/0",
				Except: []string{"128.0.0.0/1"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"0.0.0.0/1", "128.0.0.0/1 nomatch"}...),
		},
		{
			name:        "cidr: 0.0.0.0/0 and except: 0.0.0.0/1 and 128.0.0.0/1",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "0.0.0.0/0",
				Except: []string{"0.0.0.0/1", "128.0.0.0/1"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"0.0.0.0/1 nomatch", "128.0.0.0/1 nomatch"}...),
		},
		{
			name:        "cidr: 0.0.0.0/0 and except: 0.0.0.0/1 and two 128.0.0.0/1",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "0.0.0.0/0",
				Except: []string{"0.0.0.0/1", "128.0.0.0/1", "128.0.0.0/1"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"0.0.0.0/1 nomatch", "128.0.0.0/1 nomatch"}...),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ipBlockIPSet(tt.policyName, tt.namemspace, tt.direction, tt.ipBlockSetIndex, tt.ipBlockRule)
			require.Equal(t, tt.translatedIPSet, got)
		})
	}
}

func TestIPBlockRule(t *testing.T) {
	tests := []struct {
		name string
		*ipBlockInfo
		ipBlockRule     *networkingv1.IPBlock
		translatedIPSet *ipsets.TranslatedIPSet
		setInfo         policies.SetInfo
	}{
		{
			name:            "empty ipblock rule ",
			ipBlockInfo:     createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule:     nil,
			translatedIPSet: nil,
			setInfo:         policies.SetInfo{},
		},
		{
			name:        "incorrect ipblock rule with only except",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: nil,
			setInfo:         policies.SetInfo{},
		},
		{
			name:        "only cidr",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR: "172.17.0.0/16",
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16"}...),
			setInfo:         policies.NewSetInfo("test-in-ns-default-0IN", ipsets.CIDRBlocks, included, policies.SrcMatch),
		},
		{
			name:        "one cidr and one element in except",
			ipBlockInfo: createIPBlockInfo("test", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24 nomatch"}...),
			setInfo:         policies.NewSetInfo("test-in-ns-default-0IN", ipsets.CIDRBlocks, included, policies.SrcMatch),
		},
		{
			name:        "one cidr and multiple elements in except",
			ipBlockInfo: createIPBlockInfo("test-network-policy", "default", policies.Ingress, policies.SrcMatch, 0),
			ipBlockRule: &networkingv1.IPBlock{
				CIDR:   "172.17.0.0/16",
				Except: []string{"172.17.1.0/24", "172.17.2.0/24"},
			},
			translatedIPSet: ipsets.NewTranslatedIPSet("test-network-policy-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24 nomatch", "172.17.2.0/24 nomatch"}...),
			setInfo:         policies.NewSetInfo("test-network-policy-in-ns-default-0IN", ipsets.CIDRBlocks, included, policies.SrcMatch),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			translatedIPSet, setInfo := ipBlockRule(tt.policyName, tt.namemspace, tt.direction, tt.matchType, tt.ipBlockSetIndex, tt.ipBlockRule)
			require.Equal(t, tt.translatedIPSet, translatedIPSet)
			require.Equal(t, tt.setInfo, setInfo)
		})
	}
}

func TestPodSelector(t *testing.T) {
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
				ipsets.NewTranslatedIPSet("default", ipsets.Namespace),
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
				ipsets.NewTranslatedIPSet("test", ipsets.Namespace),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
				ipsets.NewTranslatedIPSet("label", ipsets.KeyLabelOfPod),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
				ipsets.NewTranslatedIPSet("labelIn:src", ipsets.KeyValueLabelOfPod),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
				ipsets.NewTranslatedIPSet("labelNotIn:src", ipsets.KeyValueLabelOfPod),
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
				ipsets.NewTranslatedIPSet("k0:v0", ipsets.KeyValueLabelOfPod),
				ipsets.NewTranslatedIPSet("k1:v10:v11", ipsets.NestedLabelOfPod, []string{"k1:v10", "k1:v11"}...),
				ipsets.NewTranslatedIPSet("k1:v10", ipsets.KeyValueLabelOfPod),
				ipsets.NewTranslatedIPSet("k1:v11", ipsets.KeyValueLabelOfPod),
				ipsets.NewTranslatedIPSet("k2", ipsets.KeyLabelOfPod),
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
			var err error
			if tt.namespace == "" {
				podSelectorIPSets, podSelectorList, err = podSelector(tt.matchType, tt.labelSelector)
			} else {
				podSelectorIPSets, podSelectorList, err = podSelectorWithNS(tt.namespace, tt.matchType, tt.labelSelector)
			}
			require.NoError(t, err)
			require.Equal(t, tt.podSelectorIPSets, podSelectorIPSets)
			require.Equal(t, tt.podSelectorList, podSelectorList)
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
				ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace, included, matchType),
			},
		},
		{
			name:      "namespaceSelector with one label in ingress",
			matchType: matchType,
			labelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"test": "",
				},
			},
			nsSelectorIPSets: []*ipsets.TranslatedIPSet{
				ipsets.NewTranslatedIPSet("test:", ipsets.KeyValueLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo("test:", ipsets.KeyValueLabelOfNamespace, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfNamespace, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfNamespace),
				ipsets.NewTranslatedIPSet("label", ipsets.KeyLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfNamespace, included, matchType),
				policies.NewSetInfo("label", ipsets.KeyLabelOfNamespace, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfNamespace),
				ipsets.NewTranslatedIPSet("labelIn:src", ipsets.KeyValueLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfNamespace, included, matchType),
				policies.NewSetInfo("labelIn:src", ipsets.KeyValueLabelOfNamespace, included, matchType),
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
				ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfNamespace),
				ipsets.NewTranslatedIPSet("labelNotIn:src", ipsets.KeyValueLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfNamespace, included, matchType),
				policies.NewSetInfo("labelNotIn:src", ipsets.KeyValueLabelOfNamespace, nonIncluded, matchType),
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
				ipsets.NewTranslatedIPSet("k0:v0", ipsets.KeyValueLabelOfNamespace),
				ipsets.NewTranslatedIPSet("k1:v10", ipsets.KeyValueLabelOfNamespace),
				ipsets.NewTranslatedIPSet("k2", ipsets.KeyLabelOfNamespace),
			},
			nsSelectorList: []policies.SetInfo{
				policies.NewSetInfo("k0:v0", ipsets.KeyValueLabelOfNamespace, included, matchType),
				policies.NewSetInfo("k1:v10", ipsets.KeyValueLabelOfNamespace, included, matchType),
				policies.NewSetInfo("k2", ipsets.KeyLabelOfNamespace, nonIncluded, matchType),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nsSelectorIPSets, nsSelectorList := nameSpaceSelector(tt.matchType, tt.labelSelector)
			require.Equal(t, tt.nsSelectorIPSets, nsSelectorIPSets)
			require.Equal(t, tt.nsSelectorList, nsSelectorList)
		})
	}
}

func TestAllowAllInternal(t *testing.T) {
	matchType := policies.SrcMatch
	tests := []struct {
		name             string
		matchType        policies.MatchType
		nsSelectorIPSets *ipsets.TranslatedIPSet
		nsSelectorList   policies.SetInfo
	}{
		{
			name:             "Allow all traffic from all namespaces in ingress",
			matchType:        matchType,
			nsSelectorIPSets: ipsets.NewTranslatedIPSet(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace),
			nsSelectorList:   policies.NewSetInfo(util.KubeAllNamespacesFlag, ipsets.KeyLabelOfNamespace, included, matchType),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nsSelectorIPSets, nsSelectorList := allowAllInternal(tt.matchType)
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
			t.Parallel()
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
				ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
			},
			acl: &policies.ACLPolicy{
				DstList: []policies.SetInfo{
					policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, matchType),
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
				ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
			},
			acl: &policies.ACLPolicy{
				DstList: []policies.SetInfo{
					policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, matchType),
				},
				Protocol: "TCP",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
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
			t.Parallel()
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
			policies.NewSetInfo("test-in-ns-default-0IN", ipsets.CIDRBlocks, included, matchType),
		},
		{
			policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfNamespace, included, matchType),
			policies.NewSetInfo("label", ipsets.KeyLabelOfNamespace, included, matchType),
		},
		{
			policies.NewSetInfo("k0:v0", ipsets.KeyValueLabelOfPod, included, matchType),
			policies.NewSetInfo("k2", ipsets.KeyLabelOfPod, nonIncluded, matchType),
			policies.NewSetInfo("k1:v10:v11", ipsets.NestedLabelOfPod, included, matchType),
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
					ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstList: []policies.SetInfo{
							policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, policies.DstDstMatch),
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
					ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList: []policies.SetInfo{
							policies.NewSetInfo("test-in-ns-default-0IN", ipsets.CIDRBlocks, included, matchType),
						},
						DstList: []policies.SetInfo{
							policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, policies.DstDstMatch),
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
					ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstList: []policies.SetInfo{
							policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, policies.DstDstMatch),
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
					ipsets.NewTranslatedIPSet("serve-tcp", ipsets.NamedPorts),
				},
				ACLs: []*policies.ACLPolicy{
					{
						PolicyID:  "azure-acl-default-serve-tcp",
						Target:    policies.Allowed,
						Direction: policies.Ingress,
						SrcList:   []policies.SetInfo{},
						DstList: []policies.SetInfo{
							policies.NewSetInfo("serve-tcp", ipsets.NamedPorts, included, policies.DstDstMatch),
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
			t.Parallel()
			for _, acl := range tt.npmNetPol.ACLs {
				acl.SrcList = setInfo
			}
			npmNetPol := &policies.NPMNetworkPolicy{
				Name:      tt.npmNetPol.Name,
				NameSpace: tt.npmNetPol.NameSpace,
			}
			err := peerAndPortRule(npmNetPol, policies.Ingress, tt.ports, setInfo)
			require.NoError(t, err)
			require.Equal(t, tt.npmNetPol, npmNetPol)
		})
	}
}

func TestIngressPolicy(t *testing.T) {
	tcp := v1.ProtocolTCP
	targetPodMatchType := policies.EitherMatch
	peerMatchType := policies.SrcMatch
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
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace),
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
					defaultDropACL("default", "serve-tcp", policies.Ingress),
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
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				RuleIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("only-ipblock-in-ns-default-0IN", ipsets.CIDRBlocks, []string{"172.17.0.0/16", "172.17.1.0/24 nomatch"}...),
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
					defaultDropACL("default", "only-ipblock", policies.Ingress),
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
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				RuleIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("peer-podselector-kay:peer-podselector-value", ipsets.KeyValueLabelOfPod),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace),
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
					defaultDropACL("default", "only-peer-podSelector", policies.Ingress),
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
					ipsets.NewTranslatedIPSet("label:src", ipsets.KeyValueLabelOfPod),
					ipsets.NewTranslatedIPSet("default", ipsets.Namespace),
				},
				PodSelectorList: []policies.SetInfo{
					policies.NewSetInfo("label:src", ipsets.KeyValueLabelOfPod, included, targetPodMatchType),
					policies.NewSetInfo("default", ipsets.Namespace, included, targetPodMatchType),
				},
				RuleIPSets: []*ipsets.TranslatedIPSet{
					ipsets.NewTranslatedIPSet("peer-nsselector-kay:peer-nsselector-value", ipsets.KeyValueLabelOfNamespace),
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
					defaultDropACL("default", "only-peer-nsSelector", policies.Ingress),
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			npmNetPol := &policies.NPMNetworkPolicy{
				Name:      tt.npmNetPol.Name,
				NameSpace: tt.npmNetPol.NameSpace,
			}
			var err error
			npmNetPol.PodSelectorIPSets, npmNetPol.PodSelectorList, err = podSelectorWithNS(npmNetPol.NameSpace, policies.EitherMatch, tt.targetSelector)
			require.NoError(t, err)
			err = ingressPolicy(npmNetPol, tt.rules)
			require.NoError(t, err)
			require.Equal(t, tt.npmNetPol, npmNetPol)
		})
	}
}
