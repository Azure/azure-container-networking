package dataplane

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/npm"
	"github.com/Azure/azure-container-networking/npm/debugTools/pb"
	"github.com/Azure/azure-container-networking/npm/util"
	"google.golang.org/protobuf/encoding/protojson"
)

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

// Input struct
type Input struct {
	Content string
	Type    InputType
}

// InputType indicates allowed typle for source and destination input
type InputType int32

const (
	// IPADDRS indicates the IP Address input type
	IPADDRS InputType = 0
	// PODNAME indicates the podname input type
	PODNAME InputType = 1
	// EXTERNAL indicates the external input type
	EXTERNAL InputType = 2
)

var ipPodMap = make(map[string]*npm.NpmPod)

// GetNetworkTuple read from node's NPM cache and iptables-save and
// returns a list of hit rules between the source and the destination in
// JSON format and a list of tuples from those rules.
func GetNetworkTuple(src, dst *Input) ([][]byte, []*Tuple, error) {
	c := &Converter{}

	allRules, err := c.GetProtobufRulesFromIptable("filter")
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get network tuple : %w", err)
	}
	return getNetworkTupleCommon(src, dst, c.NPMCache, allRules)
}

// GetNetworkTupleFile read from NPM cache and iptables-save files and
// returns a list of hit rules between the source and the destination in
// JSON format and a list of tuples from those rules.
func GetNetworkTupleFile(
	src, dst *Input,
	npmCacheFile string,
	iptableSaveFile string,
) ([][]byte, []*Tuple, error) {

	c := &Converter{}
	allRules, err := c.GetProtobufRulesFromIptableFile(util.IptablesFilterTable, npmCacheFile, iptableSaveFile)
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get network tuple : %w", err)
	}

	return getNetworkTupleCommon(src, dst, c.NPMCache, allRules)
}

// Common function.
func getNetworkTupleCommon(
	src, dst *Input,
	npmCache *NPMCache,
	allRules []*pb.RuleResponse,
) ([][]byte, []*Tuple, error) {

	for _, pod := range npmCache.PodMap {
		ipPodMap[pod.PodIP] = pod
	}

	srcPod, err := getNPMPod(src, npmCache)
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get source pod : %w", err)
	}

	dstPod, err := getNPMPod(dst, npmCache)
	if err != nil {
		return nil, nil, fmt.Errorf("error occurred during get destination pod : %w", err)
	}

	hitRules, err := getHitRules(srcPod, dstPod, allRules, npmCache)
	if err != nil {
		return nil, nil, fmt.Errorf("%w", err)
	}

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

	resTupleList := make([]*Tuple, 0)
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
	return ruleResListJSON, resTupleList, nil
}

func getNPMPod(input *Input, npmCache *NPMCache) (*npm.NpmPod, error) {
	switch input.Type {
	case PODNAME:
		if pod, ok := npmCache.PodMap[input.Content]; ok {
			return pod, nil
		}
		return nil, errInvalidInput
	case IPADDRS:
		if pod, ok := ipPodMap[input.Content]; ok {
			return pod, nil
		}
		return nil, errInvalidIPAddress
	case EXTERNAL:
		return &npm.NpmPod{}, nil
	default:
		return nil, errInvalidInput
	}
}

// GetInputType returns the type of the input for GetNetworkTuple.
func GetInputType(input string) InputType {
	if input == "External" {
		return EXTERNAL
	} else if ip := net.ParseIP(input); ip != nil {
		return IPADDRS
	} else {
		return PODNAME
	}
}

func generateTuple(src, dst *npm.NpmPod, rule *pb.RuleResponse) *Tuple {
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
		tuple.SrcIP = src.PodIP
	}
	if rule.SPort != 0 {
		tuple.SrcPort = strconv.Itoa(int(rule.SPort))
	} else {
		tuple.SrcPort = ANY
	}
	if len(rule.DstList) == 0 {
		tuple.DstIP = ANY
	} else {
		tuple.DstIP = dst.PodIP
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
	src, dst *npm.NpmPod,
	rules []*pb.RuleResponse,
	npmCache *NPMCache,
) ([]*pb.RuleResponse, error) {

	res := make([]*pb.RuleResponse, 0)
	for _, rule := range rules {
		matched := true
		for _, setInfo := range rule.SrcList {
			// evalute all match set in src
			if src.Namespace == "" {
				// internet
				matched = false
				break
			}
			matchedSource, err := evaluateSetInfo("src", setInfo, src, rule, npmCache)
			if err != nil {
				return nil, fmt.Errorf("error occurred during evaluating source's set info : %w", err)
			}
			if !matchedSource {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		for _, setInfo := range rule.DstList {
			// evaluate all match set in dst
			if dst.Namespace == "" {
				// internet
				matched = false
				break
			}
			matchedDestination, err := evaluateSetInfo("dst", setInfo, dst, rule, npmCache)
			if err != nil {
				return nil, fmt.Errorf("error occurred during evaluating destination's set info : %w", err)
			}
			if !matchedDestination {
				matched = false
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
	pod *npm.NpmPod,
	rule *pb.RuleResponse,
	npmCache *NPMCache,
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
		return false, errSetType
	}
}

func matchKEYVALUELABELOFNAMESPACE(pod *npm.NpmPod, npmCache *NPMCache, setInfo *pb.RuleResponse_SetInfo) bool {
	srcNamespace := util.NamespacePrefix + pod.Namespace
	key, expectedValue := processKeyValueLabelOfNameSpace(setInfo.Name)
	actualValue := npmCache.NsMap[srcNamespace].LabelsMap[key]
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

func matchNESTEDLABELOFPOD(pod *npm.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
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

func matchKEYLABELOFNAMESPACE(pod *npm.NpmPod, npmCache *NPMCache, setInfo *pb.RuleResponse_SetInfo) bool {
	srcNamespace := util.NamespacePrefix + pod.Namespace
	key := strings.TrimPrefix(setInfo.Name, util.NamespacePrefix)
	if _, ok := npmCache.NsMap[srcNamespace].LabelsMap[key]; ok {
		return setInfo.Included
	}
	if setInfo.Included {
		// if key does not exist but required in rule
		return false
	}
	return true
}

func matchNAMESPACE(pod *npm.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	srcNamespace := util.NamespacePrefix + pod.Namespace
	if setInfo.Name != srcNamespace || (setInfo.Name == srcNamespace && !setInfo.Included) {
		return false
	}
	return true
}

func matchKEYVALUELABELOFPOD(pod *npm.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
	key, value := processKeyValueLabelOfPod(setInfo.Name)
	if pod.Labels[key] != value || (pod.Labels[key] == value && !setInfo.Included) {
		return false
	}
	return true
}

func matchKEYLABELOFPOD(pod *npm.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
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

func matchNAMEDPORTS(pod *npm.NpmPod, setInfo *pb.RuleResponse_SetInfo, rule *pb.RuleResponse, origin string) bool {
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

func matchCIDRBLOCKS(pod *npm.NpmPod, setInfo *pb.RuleResponse_SetInfo) bool {
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
	kvList := strings.Split(kv, ":")
	key := kvList[0]
	ret := make([]string, 0)
	for _, value := range kvList[1:] {
		ret = append(ret, key+":"+value)
	}
	return ret
}
