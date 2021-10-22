package translation

import (
	"fmt"
	"io/ioutil"
	"strconv"
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

func TestPortRule(t *testing.T) {
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

// TODO(junguk): move to Policy code
func TestCidrField(t *testing.T) {
	cidr := "172.17.0.0/16"
	except := "172.17.1.0/24"
	excepts := []string{"172.17.1.0/24", "172.17.1.0/32"}
	exceptsWithSurfix := []string{"172.17.1.0/24" + util.IpsetNomatch, "172.17.1.0/32" + util.IpsetNomatch}
	tests := []struct {
		name        string
		IPBlockRule networkingv1.IPBlock
		want        []string
		wantErr     bool
	}{
		{
			name:        "empty",
			IPBlockRule: networkingv1.IPBlock{},
			want:        []string{},
		},
		{
			name: "one cidr",
			IPBlockRule: networkingv1.IPBlock{
				CIDR: cidr,
			},
			want: []string{cidr},
		},
		{
			name: "one cidr and one except",
			IPBlockRule: networkingv1.IPBlock{
				CIDR:   cidr,
				Except: []string{except},
			},
			want: []string{cidr, except + util.IpsetNomatch},
		},
		{
			name: "one cidr and multiple excepts",
			IPBlockRule: networkingv1.IPBlock{
				CIDR:   cidr,
				Except: excepts,
			},
			want: append([]string{cidr}, exceptsWithSurfix...),
		},
		{
			name: "only multiple excepts - invalid - (need to check it)",
			IPBlockRule: networkingv1.IPBlock{
				Except: []string{except},
			},
			wantErr: true,
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			policyName, namemspace, direction, ipBlockSetIndex := "test", "test", policies.Ingress, 0
			got := translator.ipBlockIPSet(policyName, namemspace, direction, ipBlockSetIndex, &tt.IPBlockRule)
			if tt.wantErr {
				t.Error("Something wrong")
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCidr(t *testing.T) {
	cidr := "172.17.0.0/16"
	except := "172.17.1.0/24"
	// excepts := []string{"172.17.1.0/24", "172.17.1.0/32"}
	// exceptsWithSurfix := []string{"172.17.1.0/24" + util.IpsetNomatch, "172.17.1.0/32" + util.IpsetNomatch}
	policyName, ns, index := "test", "test", 0
	ipsetName := policyName + "-in-ns-" + ns + "-" + strconv.Itoa(index) + "in"

	tests := []struct {
		name        string
		IPBlockRule networkingv1.IPBlock
		want        *ipsets.IPSet
		wantErr     bool
	}{
		{
			name:        "empty",
			IPBlockRule: networkingv1.IPBlock{},
			want:        nil,
		},
		{
			name: "one cidr",
			IPBlockRule: networkingv1.IPBlock{
				CIDR: cidr,
			},
			want: &ipsets.IPSet{
				Name: ipsetName,
				SetProperties: ipsets.SetProperties{
					Type: ipsets.CIDRBlocks,
					Kind: ipsets.HashSet,
				},
				IPPodKey: map[string]string{
					cidr: ipsetName,
				},
			},
		},
		{
			name: "one cidr and one except",
			IPBlockRule: networkingv1.IPBlock{
				CIDR:   cidr,
				Except: []string{except},
			},
			want: &ipsets.IPSet{
				Name: ipsetName,
				SetProperties: ipsets.SetProperties{
					Type: ipsets.CIDRBlocks,
					Kind: ipsets.HashSet,
				},
				IPPodKey: map[string]string{
					cidr:                       ipsetName,
					except + util.IpsetNomatch: ipsetName,
				},
			},
		},
		// {
		// 	name: "one cidr and multiple excepts",
		// 	IPBlockRule: networkingv1.IPBlock{
		// 		CIDR:   cidr,
		// 		Except: []string{except},
		// 	},
		// 	want: &ipsets.IPSet{
		// 		Name: ipsetName,
		// 		SetProperties: ipsets.SetProperties{
		// 			Type: ipsets.CIDRBlocks,
		// 			Kind: ipsets.HashSet,
		// 		},
		// 		IPPodKey: map[string]string{
		// 			cidr:                       ipsetName,
		// 			except + util.IpsetNomatch: ipsetName,
		// 		},
		// 	},
		// },
		{
			name: "only multiple excepts - invalid - (need to check it)",
			IPBlockRule: networkingv1.IPBlock{
				Except: []string{except},
			},
			want: nil,
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			policyName, namemspace, direction, ipBlockSetIndex := "test", "test", policies.Ingress, 0
			ipBlockIPSets := translator.ipBlockIPSet(policyName, namemspace, direction, ipBlockSetIndex, &tt.IPBlockRule)
			require.Equal(t, tt.want, ipBlockIPSets)
		})
	}
}

func TestNamedPortRule(t *testing.T) {
	namedPortStr := "serve-tcp"
	// TODO(junguk) Why it shows error
	namedPort := intstr.FromString(namedPortStr)
	type namedPortOutput struct {
		translatedIPSet *ipsets.TranslatedIPSet
		protocol        string
	}
	tcp := v1.ProtocolTCP
	tests := []struct {
		name     string
		portRule networkingv1.NetworkPolicyPort
		want     *namedPortOutput
		wantErr  bool
	}{
		{
			name:     "empty",
			portRule: networkingv1.NetworkPolicyPort{},
			want: &namedPortOutput{
				translatedIPSet: nil, // (TODO): Need to check it
				protocol:        "TCP"},
		},
		{
			name: "serve-tcp",
			portRule: networkingv1.NetworkPolicyPort{
				Protocol: &tcp,
				Port:     &namedPort,
			},

			want: &namedPortOutput{
				translatedIPSet: &ipsets.TranslatedIPSet{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
				},
				protocol: "TCP"},
		},
		{
			name: "serve-tcp without protocol field",
			portRule: networkingv1.NetworkPolicyPort{
				Port: &namedPort,
			},
			want: &namedPortOutput{
				translatedIPSet: &ipsets.TranslatedIPSet{
					Metadata: &ipsets.IPSetMetadata{
						Name: util.NamedPortIPSetPrefix + "serve-tcp",
						Type: ipsets.NamedPorts,
					},
				},
				protocol: "TCP",
			},
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			translatedIPSet, protocol := translator.namedPortRuleInfo(&tt.portRule)
			got := &namedPortOutput{
				translatedIPSet: translatedIPSet,
				protocol:        protocol,
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOnlyNamedPorts(t *testing.T) {
	policyFile := "testpolicies/named-port.yaml"
	netpol, err := readPolicyYaml(policyFile)
	if err != nil {
		t.Fatal(err)
	}

	if err != nil {
		t.Fatal(err)
	}

	npPolicy := policies.NPMNetworkPolicy{
		Name:              netpol.Name,
		NameSpace:         netpol.Namespace,
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{},
		RuleIPSets:        []*ipsets.TranslatedIPSet{},
		ACLs:              []*policies.ACLPolicy{},
		RawNP:             &networkingv1.NetworkPolicy{},
	}

	translator := &translator{}

	// ops, labelsForSpec, singleValueLabels, multiValuesLabels := translator.podSelectorIPSets(netpol.Namespace, &netpol.Spec.PodSelector, util.IptablesDstFlag)
	ops, labelsForSpec, _, _ := translator.targetPodSelectorInfo(netpol.Namespace, &netpol.Spec.PodSelector)
	dstList := translator.podSelectorRule(ops, labelsForSpec)
	for i, dst := range dstList {
		fmt.Printf("%d %+v\n", i, dst)
	}

	// #2. Get Port Information
	// only port case
	for _, rule := range netpol.Spec.Ingress {
		for _, port := range rule.Ports {
			acl := &policies.ACLPolicy{
				PolicyID:  netpol.Name, // redundant
				Target:    policies.Allowed,
				Direction: policies.Ingress,
				DstList:   dstList,
			}
			_, dstPortIPSet, protocol := translator.namedPortRule(&port)
			acl.DstList = append(acl.DstList, dstPortIPSet)
			acl.Protocol = policies.Protocol(protocol)
			npPolicy.ACLs = append(npPolicy.ACLs, acl)
		}
	}

	for _, acl := range npPolicy.ACLs {
		for i, src := range acl.SrcList {
			fmt.Printf("src %d %+v\n", i, src)
		}

		for i, src := range acl.SrcList {
			fmt.Printf("dst %d %+v\n", i, src)
		}
	}
}

// TODO(junguk): need to check expected values
func TestPodSelectorIPSets(t *testing.T) {
	tests := []struct {
		name        string
		srcSelector *metav1.LabelSelector
		wantErr     bool
	}{
		{
			name: "only match labels",
			srcSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"label": "src",
				},
			},
		},
		{
			name: "match labels and match expression with with Exists OP",
			srcSelector: &metav1.LabelSelector{
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
		},
		{
			name: "match labels and match expression with single value and In OP",
			srcSelector: &metav1.LabelSelector{
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
		},
		{
			name: "match labels and match expression with single value and NotIn OP",
			srcSelector: &metav1.LabelSelector{
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
		},
		{
			name: "match labels and match expression with multi values and In and NotExist",
			srcSelector: &metav1.LabelSelector{
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
		},
	}

	translator := &translator{}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// TODO(jungukcho) need to test this function
			//podSelectorIPSets, targetPodDstList := translator.targetPodSelector("testnamespace", tt.srcSelector, util.IptablesDstFlag)
			// for i, targetPodDst := range targetPodDstList {
			// 	fmt.Printf("%d %+v\n", i, targetPodDst)
			// }
			ops, labelsForSpec, singleValueLabels, multiValuesLabels := translator.targetPodSelectorInfo("testnamespace", tt.srcSelector)
			iptEntry := translator.IptEntryForPodSelectorIPSets("testnamespace", ops, labelsForSpec, util.IptablesDstFlag)
			fmt.Printf("ops : %v len : %d\n", ops, len(ops)) // (TODO): Need to fix it.. How? no ops -> ""? weird.. (TODO)
			fmt.Printf("labelsForSpec : %v\n", labelsForSpec)
			fmt.Printf("singleValueLabels : %v\n", singleValueLabels)
			fmt.Printf("multiValuesLabels : %v\n", multiValuesLabels)
			fmt.Printf("iptEntry: %v\n", iptEntry)
		})
	}
}

func TestOnlyPort(t *testing.T) {
	policyFile := "testpolicies/only-ports.yaml"
	netpol, err := readPolicyYaml(policyFile)

	if err != nil {
		t.Fatal(err)
	}

	npPolicy := policies.NPMNetworkPolicy{
		Name:              netpol.Name,
		NameSpace:         netpol.Namespace,
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{},
		RuleIPSets:        []*ipsets.TranslatedIPSet{},
		ACLs:              []*policies.ACLPolicy{},
		RawNP:             &networkingv1.NetworkPolicy{},
	}

	// Just need PodSelectorIPSets Information
	// 0     0 MARK       udp  --  *      *       0.0.0.0/0            0.0.0.0/0
	// udp dpt:100 match-set azure-npm-784554818 dst match-set azure-npm-1519775445 dst
	// /* ALLOW-ALL-UDP-PORT-100-TO-app:server-IN-ns-default */ MARK set 0x2000
	translator := &translator{}

	// #1. Calculate podIPEntry
	// ops, labelsForSpec, singleValueLabels, multiValuesLabels := translator.podSelectorIPSets(netpol.Namespace, &netpol.Spec.PodSelector, util.IptablesDstFlag)
	ops, labelsForSpec, _, _ := translator.targetPodSelectorInfo(netpol.Namespace, &netpol.Spec.PodSelector)
	dstList := translator.podSelectorRule(ops, labelsForSpec)
	for i, dst := range dstList {
		fmt.Printf("%d %+v\n", i, dst)
	}

	// #2. Get Port Information
	// only port case
	for _, rule := range netpol.Spec.Ingress {
		for _, port := range rule.Ports {
			acl := &policies.ACLPolicy{
				PolicyID:  netpol.Name, // redundant
				Target:    policies.Allowed,
				Direction: policies.Ingress,
				DstList:   dstList,
			}
			portInfo, protocol := translator.numericPortRule(&port)
			acl.DstPorts = portInfo
			acl.Protocol = policies.Protocol(protocol)
			npPolicy.ACLs = append(npPolicy.ACLs, acl)
		}
	}

	for _, acl := range npPolicy.ACLs {
		fmt.Printf("%+v\n", acl.SrcList)
	}
}

// To understand how iptm.IptEntry is created by using ACLPolicy (will be deleted)

// Just to get hash values (will be deleted)
func TestHashValues(t *testing.T) {
	/*
		ns-testnamespace : azure-npm-2173871756
		k0:v0 : azure-npm-901539978
		k2 : azure-npm-2503834632
		k1:v10:v11 : azure-npm-992793578
		label : azure-npm-4137097213
		label:src : azure-npm-1570928457
		app:backend : azure-npm-3038731686
		role:frontend : azure-npm-2574419033
		namedport:serve-80 : azure-npm-3050895063
		ns-netpol-4537-x : azure-npm-3024785582
		pod:a : azure-npm-3545492025
		pod:x : azure-npm-3931377262
		pod:b : azure-npm-3495159168
		pod:c : azure-npm-3511936787
		app:test : azure-npm-2817129730
		app:server : azure-npm-1519775445

		ns-ns:netpol-4537-x : azure-npm-1052046537
		ns-ns:netpol-4537-y : azure-npm-1035268918
		app:test : azure-npm-2817129730
		app:int : azure-npm-3357534811
		pod:a:x : azure-npm-4176901587 pod:a : azure-npm-3545492025
		pod:a:x : azure-npm-4176901587 pod:x : azure-npm-3931377262
		pod:b:c : azure-npm-3025643863 pod:b : azure-npm-3495159168
		pod:b:c : azure-npm-3025643863 pod:c : azure-npm-3511936787
		app:test:int : azure-npm-231489545 app:test : azure-npm-2817129730
		app:test:int : azure-npm-231489545 app:int : azure-npm-3357534811
	*/
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

// Just play with network policy (will be deleted)
/*
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
*/

/*
// Just play with network policy (will be deleted)
*/
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

// TODO check this func references and change the label and op logic
// craftPartialIptablesCommentFromSelector :- ns must be "" for namespace selectors
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

// TODO check this func references and change the label and op logic
// craftPartialIptablesCommentFromSelector :- ns must be "" for namespace selectors
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

func TestTargetPodSelector(t *testing.T) {
	translator := &translator{}

	srcSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"label": "src",
		},
	}
	/*
		targetEntry : [-m set --match-set azure-npm-2173871756 dst -m set --match-set azure-npm-1570928457 dst]
		label : [label:src]
		listLabels : map[]
	*/
	podSelectorIPSets, targetPodDstList := translator.targetPodSelector("testnamespace", srcSelector, policies.DstMatch)
	fmt.Printf("podSelectorIPSets : %v\n", podSelectorIPSets)
	fmt.Printf("targetPodDstList : %v\n", targetPodDstList)

	/*
		targetEntry : [-m set --match-set azure-npm-2173871756 dst
		-m set --match-set azure-npm-1570928457 dst
		-m set --match-set azure-npm-4137097213 dst]
		label : [label:src label]
		listLabels : map[]
	*/
	srcSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"label": "src",
		},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "label",
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}

	podSelectorIPSets, targetPodDstList = translator.targetPodSelector("testnamespace", srcSelector, policies.DstMatch)
	fmt.Printf("podSelectorIPSets : %v\n", podSelectorIPSets)
	fmt.Printf("targetPodDstList : %v\n", targetPodDstList)

	srcSelector = &metav1.LabelSelector{
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
	}

	/*
		targetEntry : [-m set --match-set azure-npm-2173871756 dst -m set --match-set azure-npm-1570928457 dst -m set --match-set azure-npm-2460817490 dst]
		label : [label:src labelIn:src]
		listLabels : map[]
	*/
	podSelectorIPSets, targetPodDstList = translator.targetPodSelector("testnamespace", srcSelector, policies.DstMatch)
	fmt.Printf("podSelectorIPSets : %v\n", podSelectorIPSets)
	fmt.Printf("targetPodDstList : %v\n", targetPodDstList)

	srcSelector = &metav1.LabelSelector{
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
	}

	/*
		targetEntry : [-m set --match-set azure-npm-2173871756 dst -m set --match-set azure-npm-1570928457 dst -m set ! --match-set azure-npm-340404361 dst]
		label : [label:src labelNotIn:src]
		listLabels : map[]
	*/
	podSelectorIPSets, targetPodDstList = translator.targetPodSelector("testnamespace", srcSelector, policies.DstMatch)
	fmt.Printf("podSelectorIPSets : %v\n", podSelectorIPSets)
	fmt.Printf("targetPodDstList : %v\n", targetPodDstList)

	srcSelector = &metav1.LabelSelector{
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
	}

	/*
		targetEntry : [-m set --match-set azure-npm-2173871756 dst -m set --match-set azure-npm-901539978 dst -m set ! --match-set azure-npm-2503834632 dst -m set --match-set azure-npm-992793578 dst]
		label : [k0:v0 k2 k1:v10 k1:v11]
		listLabels : map[k1:v10:v11:[k1:v10 k1:v11]]
	*/
	podSelectorIPSets, targetPodDstList = translator.targetPodSelector("testnamespace", srcSelector, policies.DstMatch)
	fmt.Printf("podSelectorIPSets : %v\n", podSelectorIPSets)
	fmt.Printf("targetPodDstList : %v\n", targetPodDstList)

}

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
