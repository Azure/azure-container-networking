package debug

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"google.golang.org/protobuf/encoding/protojson"
)

type TupleAndRule struct {
	Tuple *Tuple
	Rule  *pb.RuleResponse
}

// Tuple struct
type Tuple struct {
	RuleType  string `json:"ruleType"`
	Direction string `json:"direction"`
	SrcIP     string `json:"srcIP"`
	SrcPort   string `json:"srcPort"`
	DstIP     string `json:"dstIP"`
	DstPort   string `json:"dstPort"`
	Protocol  string `json:"protocol"`
}

func PrettyPrintTuples(tuples []*TupleAndRule, srcList map[string]*pb.RuleResponse_SetInfo, dstList map[string]*pb.RuleResponse_SetInfo) { //nolint: gocritic
	allowedrules := []*TupleAndRule{}
	for _, tuple := range tuples {
		if tuple.Tuple.RuleType == "ALLOWED" {
			allowedrules = append(allowedrules, tuple)
			continue
		}
		/*tuple.Tuple.Direction == "EGRESS" {
			fmt.Printf("\tProtocol: %s, Port: %s\n, Chain: %v", tuple.Tuple.Protocol, tuple.Tuple.SrcPort, tuple.Rule.Chain)
		}*/
	}

	sort.Slice(allowedrules, func(i, j int) bool {
		return allowedrules[i].Tuple.Direction == "EGRESS"
	})

	tuplechains := make(map[Tuple]string)

	fmt.Printf("Allowed:\n")
	section := ""
	for _, tuple := range allowedrules {

		if tuple.Tuple.Direction != section {
			fmt.Printf("\t%s:\n", tuple.Tuple.Direction)
			section = tuple.Tuple.Direction
		}

		t := *tuple
		if chain, ok := tuplechains[*t.Tuple]; ok {
			// doesn't exist in map
			if chain != t.Rule.Chain {
				// we've seen this tuple before with a different chain, need to print
				fmt.Printf("\t\tProtocol: %s, Port: %s, Chain: %v, Comment: %v\n", tuple.Tuple.Protocol, tuple.Tuple.DstPort, tuple.Rule.Chain, tuple.Rule.Comment)
			}
		} else {
			// we haven't seen this tuple before, print everything
			tuplechains[*t.Tuple] = t.Rule.Chain
			fmt.Printf("\t\tProtocol: %s, Port: %s, Chain: %v, Comment: %v\n", tuple.Tuple.Protocol, tuple.Tuple.DstPort, tuple.Rule.Chain, tuple.Rule.Comment)

		}

	}
	fmt.Printf("Key:\n")
	fmt.Printf("IPSets:")
	fmt.Printf("\tSource IPSets:\n")
	for i := range srcList {
		fmt.Printf("\t\tName: %s, HashedName: %s,\n", srcList[i].Name, srcList[i].HashedSetName)
	}
	fmt.Printf("\tDestination IPSets:\n")
	for i := range dstList {
		fmt.Printf("\t\tName: %s, HashedName: %s,\n", dstList[i].Name, dstList[i].HashedSetName)
	}
}

// GetNetworkTuple read from node's NPM cache and iptables-save and
// returns a list of hit rules between the source and the destination in
// JSON format and a list of tuples from those rules.
func (c *Converter) GetNetworkTuple(src, dst *common.Input, config *npmconfig.Config) ([][]byte, []*TupleAndRule, map[string]*pb.RuleResponse_SetInfo, map[string]*pb.RuleResponse_SetInfo, error) { //nolint: gocritic,lll
	allRules, err := c.GetProtobufRulesFromIptable("filter")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error occurred during get network tuple : %w", err)
	}

	// after we have all rules from the AZURE-NPM chains in the filter table, get the network tuples of src and dst

	return getNetworkTupleCommon(src, dst, c.NPMCache, allRules, c.EnableV2NPM)
}

// GetNetworkTupleFile read from NPM cache and iptables-save files and
// returns a list of hit rules between the source and the destination in
// JSON format and a list of tuples from those rules.
func (c *Converter) GetNetworkTupleFile( //nolint:gocritic
	src, dst *common.Input,
	npmCacheFile string,
	iptableSaveFile string,
) ([][]byte, []*TupleAndRule, map[string]*pb.RuleResponse_SetInfo, map[string]*pb.RuleResponse_SetInfo, error) {
	allRules, err := c.GetProtobufRulesFromIptableFile(util.IptablesFilterTable, npmCacheFile, iptableSaveFile)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error occurred during get network tuple : %w", err)
	}

	return getNetworkTupleCommon(src, dst, c.NPMCache, allRules, c.EnableV2NPM)
}

// Common function.
func getNetworkTupleCommon(
	src, dst *common.Input,
	npmCache common.GenericCache,
	allRules map[*pb.RuleResponse]struct{},
	enableV2NPM bool,
) ([][]byte, []*TupleAndRule, map[string]*pb.RuleResponse_SetInfo, map[string]*pb.RuleResponse_SetInfo, error) {

	srcPod, err := npmCache.GetPod(src)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error occurred during get source pod : %w", err)
	}

	dstPod, err := npmCache.GetPod(dst)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error occurred during get destination pod : %w", err)
	}

	// find all rules where the source pod and dest pod exist
	hitRules, srcSets, dstSets, err := getHitRules(srcPod, dstPod, allRules, npmCache, enableV2NPM)
	if err != nil {
		return nil, nil, srcSets, dstSets, fmt.Errorf("%w", err)
	}

	ruleResListJSON := make([][]byte, 0)
	m := protojson.MarshalOptions{
		Indent:          "	",
		EmitUnpopulated: true,
	}
	for _, rule := range hitRules {
		ruleJSON, err := m.Marshal(rule) // pretty print
		if err != nil {
			return nil, nil, srcSets, dstSets, fmt.Errorf("error occurred during marshalling : %w", err)
		}
		ruleResListJSON = append(ruleResListJSON, ruleJSON)
	}

	resTupleList := make([]*TupleAndRule, 0)
	for _, rule := range hitRules {
		tuple := generateTuple(srcPod, dstPod, rule)
		resTupleList = append(resTupleList, tuple)
	}
	// tupleResListJson := make([][]byte, 0)
	// for _, rule := range resTupleList {
	// 	ruleJson, err := json.MarshalIndent(rule, "", "  ")
	// 	if err != nil {
	// 		log.Fatalf("Error occurred during marshaling. Error: %s", err.Error())
	// 	}
	// 	tupleResListJson = append(tupleResListJson, ruleJson)
	// }
	return ruleResListJSON, resTupleList, srcSets, dstSets, nil
}

func generateTuple(src, dst *common.NpmPod, rule *pb.RuleResponse) *TupleAndRule {
	tuple := &Tuple{}
	if rule.Allowed {
		tuple.RuleType = "ALLOWED"
	} else {
		tuple.RuleType = "NOT ALLOWED"
	}
	switch rule.Direction {
	case pb.Direction_EGRESS:
		tuple.Direction = "EGRESS"
	case pb.Direction_INGRESS:
		tuple.Direction = "INGRESS"
	case pb.Direction_UNDEFINED:
		// not sure if this is correct
		tuple.Direction = ANY
	default:
		tuple.Direction = ANY
	}
	if len(rule.SrcList) == 0 {
		tuple.SrcIP = ANY
	} else {
		tuple.SrcIP = src.IP()
	}
	if rule.SPort != 0 {
		tuple.SrcPort = strconv.Itoa(int(rule.SPort))
	} else {
		tuple.SrcPort = ANY
	}
	if len(rule.DstList) == 0 {
		tuple.DstIP = ANY
	} else {
		tuple.DstIP = dst.IP()
	}
	if rule.DPort != 0 {
		tuple.DstPort = strconv.Itoa(int(rule.DPort))
	} else {
		tuple.DstPort = ANY
	}
	if rule.Protocol != "" {
		tuple.Protocol = rule.Protocol
	} else {
		tuple.Protocol = ANY
	}
	return &TupleAndRule{
		Tuple: tuple,
		Rule:  rule,
	}
}

func getHitRules(
	src, dst *common.NpmPod,
	rules map[*pb.RuleResponse]struct{},
	npmCache common.GenericCache,
	enableV2NPM bool,
) ([]*pb.RuleResponse, map[string]*pb.RuleResponse_SetInfo, map[string]*pb.RuleResponse_SetInfo, error) {

	res := make([]*pb.RuleResponse, 0)
	srcSets := make(map[string]*pb.RuleResponse_SetInfo, 0)
	dstSets := make(map[string]*pb.RuleResponse_SetInfo, 0)

	for rule := range rules {
		srcNamespaceMatch, err := matchNamespaceAnchorConditions("src", src, rule.GetSrcList(), rule, npmCache, enableV2NPM)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("evaluating source namespace conditions: %w", err)
		}
		if !srcNamespaceMatch {
			continue
		}
		dstNamespaceMatch, err := matchNamespaceAnchorConditions("dst", dst, rule.GetDstList(), rule, npmCache, enableV2NPM)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("evaluating destination namespace conditions: %w", err)
		}
		if !dstNamespaceMatch {
			continue
		}
		matchedSrc := false
		matchedDst := false
		// evalute all match set in src
		for _, setInfo := range rule.SrcList {
			if src.Namespace == "" {
				// internet
				break
			}

			matchedSource, err := evaluateSetInfo("src", setInfo, src, rule, npmCache, enableV2NPM)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error occurred during evaluating source's set info : %w", err)
			}
			if matchedSource {
				matchedSrc = true
				srcSets[setInfo.HashedSetName] = setInfo
				if !enableV2NPM {
					break
				}
			} else if enableV2NPM {
				matchedSrc = false
				break
			}
		}

		// evaluate all match set in dst
		for _, setInfo := range rule.DstList {
			if dst.Namespace == "" {
				// internet
				break
			}

			matchedDestination, err := evaluateSetInfo("dst", setInfo, dst, rule, npmCache, enableV2NPM)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error occurred during evaluating destination's set info : %w", err)
			}
			if matchedDestination {
				dstSets[setInfo.HashedSetName] = setInfo
				matchedDst = true
				if !enableV2NPM {
					break
				}
			} else if enableV2NPM {
				matchedDst = false
				break
			}
		}

		// conditions:
		// add if src matches and there's no dst
		// add if dst matches and there's no src
		// add if src and dst match with both src and dst specified

		if (matchedSrc && len(rule.DstList) == 0) ||
			(matchedDst && len(rule.SrcList) == 0) ||
			(matchedSrc && matchedDst) {
			res = append(res, rule)
		}
	}

	if len(res) == 0 {
		// either no hit rules or no rules at all. Both cases allow all traffic
		res = append(res, &pb.RuleResponse{Allowed: true})
	}
	return res, srcSets, dstSets, nil
}

type namespaceLabelReader interface {
	GetNamespaceLabels(namespace string) (map[string]string, bool)
}

var errNamespaceLabelsUnavailable = errors.New("diagnostic cache does not support namespace label lookup")

func namespaceLabels(npmCache common.GenericCache, namespace string) (labels map[string]string, exists bool, err error) {
	reader, ok := npmCache.(namespaceLabelReader)
	if !ok {
		return nil, false, errNamespaceLabelsUnavailable
	}
	labels, exists = reader.GetNamespaceLabels(namespace)
	return labels, exists, nil
}

// V2 selector conditions are conjunctive, whether or not an aggregate was needed.
// The converter's explicit mode keeps user-controlled v1 names outside this path.
func matchNamespaceAnchorConditions(origin string, pod *common.NpmPod, sets []*pb.RuleResponse_SetInfo, rule *pb.RuleResponse, npmCache common.GenericCache, enableV2NPM bool) (bool, error) {
	if !enableV2NPM {
		return true, nil
	}
	hasSelectorCondition := false
	for _, set := range sets {
		if set.GetType() != pb.SetType_CIDRBLOCKS && set.GetType() != pb.SetType_UNKNOWN &&
			set.GetType() != pb.SetType_NAMEDPORTS {
			hasSelectorCondition = true
			break
		}
	}
	if !hasSelectorCondition {
		return true, nil
	}

	labels, namespaceExists, err := namespaceLabels(npmCache, pod.Namespace)
	if err != nil {
		return false, err
	}
	for _, set := range sets {
		var matches bool
		switch set.GetType() {
		case pb.SetType_KEYLABELOFNAMESPACE, pb.SetType_KEYVALUELABELOFNAMESPACE:
			if set.GetName() == util.NamespaceLabelPrefix+util.KubeAllNamespacesFlagV2 {
				matches = pod.Namespace != "" && namespaceExists
			} else {
				// The converter uses the same set type for key and key:value namespace sets.
				var err error
				matches, err = matchPrefixedLabelSet(labels, set.GetName(), util.NamespaceLabelPrefix)
				if err != nil {
					return false, err
				}
			}
		case pb.SetType_NAMESPACE:
			matches = set.GetName() == util.NamespacePrefix+pod.Namespace
		case pb.SetType_KEYLABELOFPOD, pb.SetType_KEYVALUELABELOFPOD:
			var err error
			matches, err = matchPrefixedLabelSet(pod.Labels, set.GetName(), util.PodLabelPrefix)
			if err != nil {
				return false, err
			}
		case pb.SetType_NAMEDPORTS:
			if !matchNAMEDPORTS(pod, set, rule, origin) {
				return false, nil
			}
			continue
		case pb.SetType_NESTEDLABELOFPOD:
			var err error
			matches, err = matchV2NestedLabelSet(pod.Labels, set.GetName())
			if err != nil {
				return false, err
			}
		case pb.SetType_CIDRBLOCKS, pb.SetType_UNKNOWN:
			return false, fmt.Errorf("unsupported anchored set %q: %w", set.GetName(), common.ErrSetType)
		default:
			return false, fmt.Errorf("namespace condition type %v: %w", set.GetType(), common.ErrSetType)
		}
		if matches != set.GetIncluded() {
			return false, nil
		}
	}
	return true, nil
}

func matchPrefixedLabelSet(labels map[string]string, setName, prefix string) (bool, error) {
	name, ok := strings.CutPrefix(setName, prefix)
	key, value, hasValue := strings.Cut(name, ":")
	if !ok || key == "" {
		return false, fmt.Errorf("label set %q: %w", setName, common.ErrInvalidInput)
	}
	actual, exists := labels[key]
	return exists && (!hasValue || actual == value), nil
}

func matchV2NestedLabelSet(labels map[string]string, setName string) (bool, error) {
	name, ok := strings.CutPrefix(setName, util.NestedLabelPrefix)
	parts := strings.Split(name, util.IpsetLabelDelimter)
	// V2 encodes policyKey:labelKey:value...; neither identity can contain ':'.
	if !ok || len(parts) < 4 || parts[0] == "" || parts[1] == "" {
		return false, fmt.Errorf("nested label set %q: %w", setName, common.ErrInvalidInput)
	}
	actual, exists := labels[parts[1]]
	if !exists {
		return false, nil
	}
	for _, expected := range parts[2:] {
		if actual == expected {
			return true, nil
		}
	}
	return false, nil
}

// evalute an ipset to find out whether the pod's attributes match with the set
func evaluateSetInfo(
	origin string,
	setInfo *pb.RuleResponse_SetInfo,
	pod *common.NpmPod,
	rule *pb.RuleResponse,
	npmCache common.GenericCache,
	enableV2NPM bool,
) (bool, error) {

	switch setInfo.Type {
	case pb.SetType_KEYVALUELABELOFNAMESPACE:
		if enableV2NPM {
			return matchKEYLABELOFNAMESPACE(pod, npmCache, setInfo, true)
		}
		return matchKEYVALUELABELOFNAMESPACE(pod, npmCache, setInfo), nil
	case pb.SetType_NESTEDLABELOFPOD:
		if enableV2NPM {
			matches, err := matchV2NestedLabelSet(pod.Labels, setInfo.GetName())
			if err != nil {
				return false, err
			}
			return matches == setInfo.GetIncluded(), nil
		}
		return matchNESTEDLABELOFPOD(pod, setInfo), nil
	case pb.SetType_KEYLABELOFNAMESPACE:
		return matchKEYLABELOFNAMESPACE(pod, npmCache, setInfo, enableV2NPM)
	case pb.SetType_NAMESPACE:
		return matchNAMESPACE(pod, setInfo), nil
	case pb.SetType_KEYVALUELABELOFPOD:
		if enableV2NPM {
			matches, err := matchPrefixedLabelSet(pod.Labels, setInfo.GetName(), util.PodLabelPrefix)
			if err != nil {
				return false, err
			}
			return matches == setInfo.GetIncluded(), nil
		}
		return matchKEYVALUELABELOFPOD(pod, setInfo), nil
	case pb.SetType_KEYLABELOFPOD:
		if enableV2NPM {
			matches, err := matchPrefixedLabelSet(pod.Labels, setInfo.GetName(), util.PodLabelPrefix)
			if err != nil {
				return false, err
			}
			return matches == setInfo.GetIncluded(), nil
		}
		return matchKEYLABELOFPOD(pod, setInfo), nil
	case pb.SetType_NAMEDPORTS:
		return matchNAMEDPORTS(pod, setInfo, rule, origin), nil
	case pb.SetType_CIDRBLOCKS:
		return matchCIDRBLOCKS(pod, setInfo), nil
	default:
		return false, common.ErrSetType
	}
}

func matchKEYVALUELABELOFNAMESPACE(pod *common.NpmPod, npmCache common.GenericCache, setInfo *pb.RuleResponse_SetInfo) bool {
	srcNamespace := util.NamespacePrefix + pod.Namespace
	key, expectedValue := processKeyValueLabelOfNameSpace(setInfo.Name)
	actualValue := npmCache.GetNamespaceLabel(srcNamespace, key)
	if expectedValue != actualValue {
		// if the value is required but does not match
		if setInfo.Included {
			return false
		}
	} else {
		if !setInfo.Included {
			return false
		}
	}

	return true
}

func matchNESTEDLABELOFPOD(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	// a function to split the key and the values and then combine the key with each value
	// return list of key value pairs which are keyvaluelabel of pod
	// one match then break
	kvList := processNestedLabelOfPod(setInfo.Name)
	hasOneKeyValuePair := false
	for _, kvPair := range kvList {
		key, value := processKeyValueLabelOfPod(kvPair)
		if pod.Labels[key] == value {
			if !setInfo.Included {
				return false
			}
			hasOneKeyValuePair = true
			break
		}
	}
	if !hasOneKeyValuePair && setInfo.Included {
		return false
	}

	return true
}

func matchKEYLABELOFNAMESPACE(pod *common.NpmPod, npmCache common.GenericCache, setInfo *pb.RuleResponse_SetInfo, enableV2NPM bool) (bool, error) {
	if enableV2NPM {
		labels, namespaceExists, err := namespaceLabels(npmCache, pod.Namespace)
		if err != nil {
			return false, err
		}
		if setInfo.GetName() == util.NamespaceLabelPrefix+util.KubeAllNamespacesFlagV2 {
			return setInfo.GetIncluded() == (pod.Namespace != "" && namespaceExists), nil
		}
		matches, err := matchPrefixedLabelSet(labels, setInfo.GetName(), util.NamespaceLabelPrefix)
		if err != nil {
			return false, err
		}
		return matches == setInfo.GetIncluded(), nil
	}
	srcNamespace := pod.Namespace
	key := strings.Split(strings.TrimPrefix(setInfo.Name, util.NamespaceLabelPrefix), ":")
	included := npmCache.GetNamespaceLabel(srcNamespace, key[0])
	if included != "" && included == key[1] {
		return setInfo.GetIncluded(), nil
	}
	if setInfo.Included {
		// if key does not exist but required in rule
		return false, nil
	}
	return true, nil
}

func matchNAMESPACE(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	srcNamespace := util.NamespacePrefix + pod.Namespace
	if setInfo.Name != srcNamespace || (setInfo.Name == srcNamespace && !setInfo.Included) {
		return false
	}
	return true
}

func matchKEYVALUELABELOFPOD(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	key, value := processKeyValueLabelOfPod(setInfo.Name)
	if pod.Labels[key] != value || (pod.Labels[key] == value && !setInfo.Included) {
		return false
	}
	log.Printf("matched key value label of pod")
	return true
}

func matchKEYLABELOFPOD(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	key := setInfo.Name
	if _, ok := pod.Labels[key]; ok {
		return setInfo.Included
	}
	if setInfo.Included {
		// if key does not exist but required in rule
		return false
	}

	return true
}

func matchNAMEDPORTS(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo, rule *pb.RuleResponse, origin string) bool {
	portname := strings.TrimPrefix(setInfo.Name, util.NamedPortIPSetPrefix)
	for _, namedPort := range pod.ContainerPorts {
		if namedPort.Name == portname {
			if !setInfo.Included {
				return false
			}
			if rule.Protocol != "" && rule.Protocol != strings.ToLower(string(namedPort.Protocol)) {
				return false
			}
			if rule.Protocol == "" {
				rule.Protocol = strings.ToLower(string(namedPort.Protocol))
			}
			if origin == "src" {
				rule.SPort = namedPort.ContainerPort
			} else {
				rule.DPort = namedPort.ContainerPort
			}

			return true
		}
	}
	return false
}

func matchCIDRBLOCKS(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	matched := false
	for _, entry := range setInfo.Contents {
		entrySplitted := strings.Split(entry, " ")
		if len(entrySplitted) > 1 { // nomatch condition. i.e [172.17.1.0/24 nomatch]
			_, ipnet, _ := net.ParseCIDR(strings.TrimSpace(entrySplitted[0]))
			podIP := net.ParseIP(pod.PodIP)
			if ipnet.Contains(podIP) {
				matched = false
				break
			}
		} else {
			_, ipnet, _ := net.ParseCIDR(strings.TrimSpace(entrySplitted[0]))
			podIP := net.ParseIP(pod.PodIP)
			if ipnet.Contains(podIP) {
				matched = true
			}
		}
	}

	return matched
}

func processKeyValueLabelOfNameSpace(kv string) (string, string) {
	str := strings.TrimPrefix(kv, util.NamespacePrefix)
	ret := strings.Split(str, ":")
	return ret[0], ret[1]
}

func processKeyValueLabelOfPod(kv string) (string, string) {
	ret := strings.Split(kv, ":")
	return ret[0], ret[1]
}

func processNestedLabelOfPod(kv string) []string {
	str := strings.TrimPrefix(kv, util.NestedLabelPrefix)
	kvList := strings.Split(str, ":")
	key := kvList[0]
	ret := make([]string, 0)
	for _, value := range kvList[1:] {
		ret = append(ret, key+":"+value)
	}
	return ret
}
