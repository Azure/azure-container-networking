package debug

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	npmconfig "github.com/Azure/azure-container-networking/npm/config"
	"github.com/Azure/azure-container-networking/npm/http/api"
	common "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"google.golang.org/protobuf/encoding/protojson"
)

// GetNetworkTuple read from node's NPM cache and iptables-save and
// returns a list of hit rules between the source and the destination in
// JSON format and a list of tuples from those rules.
func GetNetworkTuple(src, dst *common.Input, config *npmconfig.Config) ([][]byte, []*common.Tuple, error) {
	c := &Converter{
		NPMDebugEndpointHost: "http://localhost",
		NPMDebugEndpointPort: api.DefaultHttpPort,
		EnableV2NPM:          config.Toggles.EnableV2NPM, // todo: pass this a different way than param to this
	}

	allRules, err := c.GetProtobufRulesFromIptable("filter")
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get network tuple : %w", err)
	}

	// after we have all rules from the AZURE-NPM chains in the filter table, get the network tuples of src and dst

	log.Printf("Cache: %+v", c.NPMCache)
	log.Printf("allRules %+v", allRules)
	return getNetworkTupleCommon(src, dst, c.NPMCache, allRules)
}

// GetNetworkTupleFile read from NPM cache and iptables-save files and
// returns a list of hit rules between the source and the destination in
// JSON format and a list of tuples from those rules.
func GetNetworkTupleFile(
	src, dst *common.Input,
	npmCacheFile string,
	iptableSaveFile string,
) ([][]byte, []*common.Tuple, error) {

	c := &Converter{}
	allRules, err := c.GetProtobufRulesFromIptableFile(util.IptablesFilterTable, npmCacheFile, iptableSaveFile)
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get network tuple : %w", err)
	}

	return getNetworkTupleCommon(src, dst, c.NPMCache, allRules)
}

// Common function.
func getNetworkTupleCommon(
	src, dst *common.Input,
	npmCache common.Cache,
	allRules []*pb.RuleResponse,
) ([][]byte, []*common.Tuple, error) {

	srcPod, err := npmCache.GetPod(src)
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get source pod : %w", err)
	}

	log.Printf("sourcepod: %+v", srcPod)

	dstPod, err := npmCache.GetPod(dst)
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get destination pod : %w", err)
	}

	log.Printf("dstpod: %+v", dstPod)

	// find all rules where the source pod and dest pod exist
	hitRules, err := getHitRules(srcPod, dstPod, allRules, npmCache)
	if err != nil {
		return nil, nil, fmt.Errorf("%w", err)
	}

	log.Printf("hitrules %+v", hitRules)

	ruleResListJSON := make([][]byte, 0)
	m := protojson.MarshalOptions{
		Indent: "	",
		EmitUnpopulated: true,
	}
	for _, rule := range hitRules {
		ruleJSON, err := m.Marshal(rule) // pretty print
		if err != nil {
			return nil, nil, fmt.Errorf("error occurred during marshalling : %w", err)
		}
		ruleResListJSON = append(ruleResListJSON, ruleJSON)
	}

	resTupleList := make([]*common.Tuple, 0)
	for _, rule := range hitRules {
		log.Printf("generating tuples for rule %+v", rule)
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
	return ruleResListJSON, resTupleList, nil
}

// GetInputType returns the type of the input for GetNetworkTuple.
func GetInputType(input string) common.InputType {
	if input == "External" {
		return common.EXTERNAL
	} else if ip := net.ParseIP(input); ip != nil {
		return common.IPADDRS
	} else {
		return common.PODNAME
	}
}

func generateTuple(src, dst *common.NpmPod, rule *pb.RuleResponse) *common.Tuple {
	tuple := &common.Tuple{}
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
	return tuple
}

func getHitRules(
	src, dst *common.NpmPod,
	rules []*pb.RuleResponse,
	npmCache common.Cache,
) ([]*pb.RuleResponse, error) {

	res := make([]*pb.RuleResponse, 0)

	for _, rule := range rules {
		matched := false
		log.Printf("evaluating rule if hit: %+v", rule)
		// evalute all match set in src
		for _, setInfo := range rule.SrcList {
			if src.Namespace == "" {
				// internet
				matched = false
				break
			}

			log.Printf("checking if set %+v in src list rules %+v", setInfo, rule.Chain)
			matchedSource, err := evaluateSetInfo("src", setInfo, src, rule, npmCache)
			if err != nil {
				return nil, fmt.Errorf("error occurred during evaluating source's set info : %w", err)
			}
			if matchedSource {
				matched = true
				break
			}
		}

		// evaluate all match set in dst
		for _, setInfo := range rule.DstList {
			if dst.Namespace == "" {
				// internet
				matched = false
				break
			}

			log.Printf("checking if set [%+v] in dst list rules [%+v]", setInfo, rule.Chain)
			matchedDestination, err := evaluateSetInfo("dst", setInfo, dst, rule, npmCache)
			if err != nil {
				return nil, fmt.Errorf("error occurred during evaluating destination's set info : %w", err)
			}
			if matchedDestination {
				matched = true
				break
			}
		}
		if matched {
			res = append(res, rule)
		}
	}
	if len(res) == 0 {
		// either no hit rules or no rules at all. Both cases allow all traffic
		res = append(res, &pb.RuleResponse{Allowed: true})
	}
	return res, nil
}

// evalute an ipset to find out whether the pod's attributes match with the set
func evaluateSetInfo(
	origin string,
	setInfo *pb.RuleResponse_SetInfo,
	pod *common.NpmPod,
	rule *pb.RuleResponse,
	npmCache common.Cache,
) (bool, error) {

	switch setInfo.Type {
	case pb.SetType_KEYVALUELABELOFNAMESPACE:
		return matchKEYVALUELABELOFNAMESPACE(pod, npmCache, setInfo), nil
	case pb.SetType_NESTEDLABELOFPOD:
		return matchNESTEDLABELOFPOD(pod, setInfo), nil
	case pb.SetType_KEYLABELOFNAMESPACE:
		return matchKEYLABELOFNAMESPACE(pod, npmCache, setInfo), nil
	case pb.SetType_NAMESPACE:
		return matchNAMESPACE(pod, setInfo), nil
	case pb.SetType_KEYVALUELABELOFPOD:
		return matchKEYVALUELABELOFPOD(pod, setInfo), nil
	case pb.SetType_KEYLABELOFPOD:
		return matchKEYLABELOFPOD(pod, setInfo), nil
	case pb.SetType_NAMEDPORTS:
		return matchNAMEDPORTS(pod, setInfo, rule, origin), nil
	case pb.SetType_CIDRBLOCKS:
		return matchCIDRBLOCKS(pod, setInfo), nil
	default:
		return false, common.ErrSetType
	}
}

func matchKEYVALUELABELOFNAMESPACE(pod *common.NpmPod, npmCache common.Cache, setInfo *pb.RuleResponse_SetInfo) bool {
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

	log.Printf("matched key value setname %s,  label of namespace %s, expected, %s, actual %s", setInfo.HashedSetName, srcNamespace, expectedValue, actualValue)
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

	log.Printf("matched nested label of pod on setinfo %s", setInfo.Name)
	return true
}

func matchKEYLABELOFNAMESPACE(pod *common.NpmPod, npmCache common.Cache, setInfo *pb.RuleResponse_SetInfo) bool {
	srcNamespace := util.NamespacePrefix + pod.Namespace
	key := strings.TrimPrefix(setInfo.Name, util.NamespaceLabelPrefix)
	included := npmCache.GetNamespaceLabel(srcNamespace, key)
	if included != "" {
		return setInfo.Included
	}
	if setInfo.Included {
		// if key does not exist but required in rule
		return false
	}
	log.Printf("matched key label of namespace, setname %s, namespace %s, key %s", setInfo.HashedSetName, srcNamespace, key)
	return true
}

func matchNAMESPACE(pod *common.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {

	srcNamespace := util.NamespacePrefix + pod.Namespace

	log.Printf("checking namespace %s with set name %s", srcNamespace, setInfo.Name)

	if setInfo.Name != srcNamespace || (setInfo.Name == srcNamespace && !setInfo.Included) {
		log.Printf("pod namespace %s did not match set %s", pod.Namespace, setInfo.Name)
		return false
	}
	log.Printf("it matched namespace")
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
	log.Printf("matched key label of pod")
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

			log.Printf("matched named ports")
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

	log.Printf("matched cidr")
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
	kvList := strings.Split(kv, ":")
	key := kvList[0]
	ret := make([]string, 0)
	for _, value := range kvList[1:] {
		ret = append(ret, key+":"+value)
	}
	return ret
}
