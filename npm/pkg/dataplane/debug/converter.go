package debug

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/http/api"
	npmcommon "github.com/Azure/azure-container-networking/npm/pkg/controlplane/controllers/common"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	NPMIPtable "github.com/Azure/azure-container-networking/npm/pkg/dataplane/iptables"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/parse"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/pb"
	"github.com/Azure/azure-container-networking/npm/pkg/models"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/pkg/errors"
)

var (
	ErrUnknownSetType = fmt.Errorf("unknown set type")
	EgressChain       = "AZURE-NPM-EGRESS"
	EgressChainPrefix = EgressChain + "-"

	IngressChain       = "AZURE-NPM-INGRESS"
	IngressChainPrefix = IngressChain + "-"
)

// Converter struct
type Converter struct {
	NPMDebugEndpointHost string
	NPMDebugEndpointPort string
	Parser               parse.IPTablesParser
	ListMap              map[string]string // key: hash(value), value: one of namespace, label of namespace, multiple values
	SetMap               map[string]string // key: hash(value), value: one of label of pods, cidr, namedport
	AzureNPMChains       map[string]bool
	NPMCache             npmcommon.GenericCache
	EnableV2NPM          bool
}

// NpmCacheFromFile initialize NPM cache from file.
func (c *Converter) NpmCacheFromFile(npmCacheJSONFile string) error {
	byteArray, err := os.ReadFile(npmCacheJSONFile)
	if err != nil {
		return fmt.Errorf("failed to read %s file : %w", npmCacheJSONFile, err)
	}

	err = c.getCacheFromBytes(byteArray)
	if err != nil {
		return errors.Wrap(err, "failed to get cache from file")
	}

	return nil
}

// NpmCache initialize NPM cache from node.
func (c *Converter) NpmCache() error {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		fmt.Sprintf("%v:%v%v", c.NPMDebugEndpointHost, c.NPMDebugEndpointPort, api.NPMMgrPath),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create http request : %w", err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to request NPM Cache : %w", err)
	}
	defer resp.Body.Close()
	byteArray, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response's data : %w", err)
	}

	err = c.getCacheFromBytes(byteArray)
	if err != nil {
		return errors.Wrapf(err, "failed to get cache from debug http endpoint")
	}

	return nil
}

// Hello Time Traveler:
// This is the client end of the debug dragons den. For issues related to marshaling,
// please refer to the custom marshaling that happens in npm/npm.go
// best of luck
func (c *Converter) getCacheFromBytes(byteArray []byte) error {
	m := map[models.CacheKey]json.RawMessage{}
	if c.EnableV2NPM {
		cache := &npmcommon.Cache{}
		if err := json.Unmarshal(byteArray, &m); err != nil {
			return errors.Wrapf(err, "failed to unmarshal into v2 cache map")
		}

		if err := json.Unmarshal(m[models.NsMap], &cache.NsMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal nsmap into v2 cache")
		}

		if err := json.Unmarshal(m[models.PodMap], &cache.PodMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal podmap into v2 cache")
		}

		if err := json.Unmarshal(m[models.SetMap], &cache.SetMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal setmap into v2 cache")
		}

		c.NPMCache = cache

	} else {
		cache := &npmcommon.Cache{}
		if err := json.Unmarshal(byteArray, &m); err != nil {
			return errors.Wrapf(err, "failed to unmarshal into v1 cache map")
		}

		if err := json.Unmarshal(m[models.NsMap], &cache.NsMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal nsmap into v1 cache map")
		}

		if err := json.Unmarshal(m[models.PodMap], &cache.PodMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal podmap into v1 cache")
		}

		if err := json.Unmarshal(m[models.SetMap], &cache.SetMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal setmap into v1 cache")
		}

		if err := json.Unmarshal(m[models.ListMap], &cache.ListMap); err != nil {
			return errors.Wrapf(err, "failed to unmarshal listmap into v1 cache")
		}

		c.NPMCache = cache
	}

	return nil
}

// Initialize converter from file.
func (c *Converter) initConverterFile(npmCacheJSONFile string) error {
	err := c.NpmCacheFromFile(npmCacheJSONFile)
	if err != nil {
		return fmt.Errorf("error occurred during initialize converter from file: %w", err)
	}
	c.initConverterMaps()
	return nil
}

// Initialize converter from node.
func (c *Converter) InitConverter() error {
	err := c.NpmCache()
	if err != nil {
		return fmt.Errorf("error occurred during initialize converter : %w", err)
	}
	c.initConverterMaps()

	c.Parser = parse.IPTablesParser{
		IOShim: common.NewIOShim(),
	}

	return nil
}

// Initialize all converter's maps.
func (c *Converter) initConverterMaps() {
	c.AzureNPMChains = make(map[string]bool)
	for _, chain := range AzureNPMChains {
		c.AzureNPMChains[chain] = true
	}

	c.ListMap = c.NPMCache.GetListMap()
	c.SetMap = c.NPMCache.GetSetMap()
}

func (c *Converter) isAzureNPMChain(chain string) bool {
	if c.EnableV2NPM {
		if strings.HasPrefix(chain, "AZURE-NPM") {
			return true
		}
	} else {
		return c.AzureNPMChains[chain]
	}
	return false
}

/*
// Convert list of protobuf rules to list of JSON rules.
func (c *Converter) jsonRuleList(pbRules []*pb.RuleResponse) ([][]byte, error) {
	ruleResListJSON := make([][]byte, 0)
	m := protojson.MarshalOptions{
		Indent:          "  ",
		EmitUnpopulated: true,
	}
	for _, rule := range pbRules {
		ruleJSON, err := m.Marshal(rule) // pretty print
		if err != nil {
			return nil, fmt.Errorf("error occurred during marshaling : %w", err)
		}
		ruleResListJSON = append(ruleResListJSON, ruleJSON)
	}
	return ruleResListJSON, nil
}
*/

// GetProtobufRulesFromIptableFile returns a list of protobuf rules from npmCache and iptable-save files.
func (c *Converter) GetProtobufRulesFromIptableFile(
	tableName string,
	npmCacheFile string,
	iptableSaveFile string,
) (map[*pb.RuleResponse]struct{}, error) {

	err := c.initConverterFile(npmCacheFile)
	if err != nil {
		return nil, fmt.Errorf("error occurred during getting protobuf rules from iptables from file: %w", err)
	}

	ipTable, err := parse.IptablesFile(tableName, iptableSaveFile)
	if err != nil {
		return nil, fmt.Errorf("error occurred during parsing iptables : %w", err)
	}
	ruleResList, err := c.pbRuleList(ipTable)
	if err != nil {
		return nil, fmt.Errorf("error occurred during getting protobuf rules from iptables pb rule list: %w", err)
	}

	return ruleResList, nil
}

// GetProtobufRulesFromIptable returns a list of protobuf rules from node.
func (c *Converter) GetProtobufRulesFromIptable(tableName string) (map[*pb.RuleResponse]struct{}, error) {
	err := c.InitConverter()
	if err != nil {
		return nil, fmt.Errorf("error occurred during getting protobuf rules from iptables : %w", err)
	}

	ipTable, err := parse.Iptables(tableName)
	if err != nil {
		return nil, fmt.Errorf("error occurred during parsing iptables : %w", err)
	}

	ruleResList, err := c.pbRuleList(ipTable)
	if err != nil {
		return nil, fmt.Errorf("error occurred during getting protobuf rules from iptables : %w", err)
	}

	return ruleResList, nil
}

// Create a list of protobuf rules from iptable.
func (c *Converter) pbRuleList(ipTable *NPMIPtable.Table) (map[*pb.RuleResponse]struct{}, error) {
	allRulesInNPMChains := make(map[*pb.RuleResponse]struct{}, 0)

	// iterate through all chains in the filter table
	for _, v := range ipTable.Chains {
		if c.isAzureNPMChain(v.Name) {

			// can skip this chain in V2 since it's an accept
			if c.EnableV2NPM && (strings.HasPrefix(v.Name, "AZURE-NPM-INGRESS-ALLOW-MARK") || (strings.HasPrefix(v.Name, "AZURE-NPM-ACCEPT"))) {
				continue
			}

			rulesFromChain, err := c.getRulesFromChain(v)
			if err != nil {
				return nil, fmt.Errorf("error occurred during getting protobuf rule list : %w", err)
			}
			/*
				if strings.HasPrefix("AZURE-NPM-EGRESS") {
					for i := range rulesFromChain {
						rulesFromChain[i].SrcList =
					}
				}
			*/
			for _, rule := range rulesFromChain {
				allRulesInNPMChains[rule] = struct{}{}
			}
		}
	}

	if c.EnableV2NPM {
		parentRules := make([]*pb.RuleResponse, 0)
		for childRule := range allRulesInNPMChains {

			// if rule is a string-int, we need to find the parent jump
			// to add the src for egress and dst for ingress
			if strings.HasPrefix(childRule.Chain, EgressChainPrefix) {
				for parentRule := range allRulesInNPMChains {
					if strings.HasPrefix(parentRule.Chain, EgressChain) && parentRule.JumpTo == childRule.Chain {
						childRule.SrcList = append(childRule.SrcList, parentRule.SrcList...)
						childRule.Comment = parentRule.Comment
						parentRules = append(parentRules, parentRule)
					}
				}
			}
			if strings.HasPrefix(childRule.Chain, IngressChainPrefix) {
				for parentRule := range allRulesInNPMChains {
					if strings.HasPrefix(parentRule.Chain, IngressChain) && parentRule.JumpTo == childRule.Chain {
						childRule.DstList = append(childRule.DstList, parentRule.DstList...)
						childRule.Comment = parentRule.Comment
						parentRules = append(parentRules, parentRule)
					}
				}
			}
		}
		for _, parentRule := range parentRules {
			delete(allRulesInNPMChains, parentRule)
		}
	}

	return allRulesInNPMChains, nil
}

func (c *Converter) getRulesFromChain(iptableChain *NPMIPtable.Chain) ([]*pb.RuleResponse, error) {
	rules := make([]*pb.RuleResponse, 0)
	// loop through each chain, if it has a jump, follow that jump
	// loop through rules in that jumped chain

	for _, v := range iptableChain.Rules {
		rule := &pb.RuleResponse{}
		rule.Chain = iptableChain.Name
		rule.Protocol = v.Protocol

		if c.EnableV2NPM {
			// chain name has to end in hash np for it to determine if allow or drop
			// ignore jumps from parent AZURE-NPM
			switch v.Target.Name {
			case util.IptablesAzureIngressAllowMarkChain:
				rule.Allowed = true

			case util.IptablesAzureAcceptChain:
				rule.Allowed = true
			default:
				// ignore other targets
				rule.Allowed = false
			}
		} else {
			switch v.Target.Name {
			case util.IptablesMark:
				rule.Allowed = true
			case util.IptablesDrop:
				rule.Allowed = false
			default:
				// ignore other targets
				continue
			}
		}

		rule.Direction = c.getRuleDirection(iptableChain.Name)

		err := c.getModulesFromRule(v.Modules, rule)
		if err != nil {
			return nil, fmt.Errorf("error occurred during getting rules from chain : %w", err)
		}

		if v.Target != nil {
			rule.JumpTo = v.Target.Name
		}

		/*
			for _, module := range v.Modules {
				if module.Verb
			}
		*/

		rules = append(rules, rule)
	}

	return rules, nil
}

func (c *Converter) getRuleDirection(iptableChainName string) pb.Direction {
	if strings.Contains(iptableChainName, "EGRESS") {
		return pb.Direction_EGRESS
	} else if strings.Contains(iptableChainName, "INGRESS") {
		return pb.Direction_INGRESS
	}
	return pb.Direction_UNDEFINED
}

func (c *Converter) getSetType(name string, m string) pb.SetType {
	if m == "ListMap" { // labels of namespace
		if strings.Contains(name, util.IpsetLabelDelimter) {
			if strings.Count(name, util.IpsetLabelDelimter) > 1 {
				return pb.SetType_NESTEDLABELOFPOD
			}
			return pb.SetType_KEYVALUELABELOFNAMESPACE
		}
		return pb.SetType_KEYLABELOFNAMESPACE
	}
	if strings.HasPrefix(name, util.NamespacePrefix) {
		return pb.SetType_NAMESPACE
	}
	if strings.HasPrefix(name, util.NamedPortIPSetPrefix) {
		return pb.SetType_NAMEDPORTS
	}
	if strings.Contains(name, util.IpsetLabelDelimter) {
		return pb.SetType_KEYVALUELABELOFPOD
	}
	matcher.Match([]byte(name))
	if matched := matcher.Match([]byte(name)); matched {
		return pb.SetType_CIDRBLOCKS
	}
	return pb.SetType_KEYLABELOFPOD
}

func (c *Converter) getSetTypeV2(name string) (pb.SetType, ipsets.SetKind) {
	var settype pb.SetType
	var setmetadata ipsets.IPSetMetadata

	switch {
	case strings.HasPrefix(name, util.CIDRPrefix):
		settype = pb.SetType_CIDRBLOCKS
		setmetadata.Type = ipsets.CIDRBlocks
	case strings.HasPrefix(name, util.NamespacePrefix):
		settype = pb.SetType_NAMESPACE
		setmetadata.Type = ipsets.Namespace
	case strings.HasPrefix(name, util.NamedPortIPSetPrefix):
		settype = pb.SetType_NAMEDPORTS
		setmetadata.Type = ipsets.NamedPorts
	case strings.HasPrefix(name, util.PodLabelPrefix):
		settype = pb.SetType_KEYLABELOFPOD // could also be KeyValueLabelOfPod
		setmetadata.Type = ipsets.KeyLabelOfPod
	case strings.HasPrefix(name, util.NamespaceLabelPrefix):
		settype = pb.SetType_KEYLABELOFNAMESPACE
		setmetadata.Type = ipsets.KeyLabelOfNamespace
	case strings.HasPrefix(name, util.NestedLabelPrefix):
		settype = pb.SetType_NESTEDLABELOFPOD
		setmetadata.Type = ipsets.NestedLabelOfPod
	default:
		log.Printf("set [%s] unknown settype", name)
		settype = pb.SetType_UNKNOWN
		setmetadata.Type = ipsets.UnknownType
	}

	return settype, setmetadata.GetSetKind()
}

func (c *Converter) getModulesFromRule(moduleList []*NPMIPtable.Module, ruleRes *pb.RuleResponse) error {
	ruleRes.SrcList = make([]*pb.RuleResponse_SetInfo, 0)
	ruleRes.DstList = make([]*pb.RuleResponse_SetInfo, 0)
	ruleRes.UnsortedIpset = make(map[string]string)

	for _, module := range moduleList {
		switch module.Verb {
		case "set":
			// set module
			OptionValueMap := module.OptionValueMap
			for option, values := range OptionValueMap {
				switch option {
				case "match-set":
					setInfo := &pb.RuleResponse_SetInfo{}

					// will populate the setinfo and add to ruleRes
					err := c.populateSetInfo(setInfo, values, ruleRes)
					if err != nil {
						return fmt.Errorf("error occurred during getting modules from rules : %w", err)
					}
					setInfo.Included = true

				case "not-match-set":
					setInfo := &pb.RuleResponse_SetInfo{}

					// will populate the setinfo and add to ruleRes
					err := c.populateSetInfo(setInfo, values, ruleRes)
					if err != nil {
						return fmt.Errorf("error occurred during getting modules from rules : %w", err)
					}
					setInfo.Included = false
				default:
					// todo add warning log
					log.Printf("%v option have not been implemented\n", option)
					continue
				}
			}

		case "tcp", "udp":
			OptionValueMap := module.OptionValueMap
			for k, v := range OptionValueMap {
				if k == "dport" {
					portNum, _ := strconv.ParseInt(v[0], Base, Bitsize)
					ruleRes.DPort = int32(portNum)
				} else {
					portNum, _ := strconv.ParseInt(v[0], Base, Bitsize)
					ruleRes.SPort = int32(portNum)
				}
			}
		case util.IptablesCommentModuleFlag:
			ruleRes.Comment = fmt.Sprintf("%+v", module.OptionValueMap[util.IptablesCommentModuleFlag])
		default:
			continue
		}
	}
	return nil
}

func (c *Converter) populateSetInfo(setInfo *pb.RuleResponse_SetInfo, values []string, ruleRes *pb.RuleResponse) error {

	ipsetHashedName := values[0]
	ipsetOrigin := values[1]
	setInfo.HashedSetName = ipsetHashedName

	if c.EnableV2NPM {
		setInfo.Name = c.SetMap[ipsetHashedName]
		settype, _ := c.getSetTypeV2(setInfo.Name)
		if settype == pb.SetType_UNKNOWN {
			return errors.Wrapf(ErrUnknownSetType, "unknown set type for set: %s", setInfo.Name)
		}

		setInfo.Type = settype
	} else {
		if v, ok := c.ListMap[ipsetHashedName]; ok {
			setInfo.Name = v
			setInfo.Type = c.getSetType(v, "ListMap")
		} else if v, ok := c.SetMap[ipsetHashedName]; ok {
			setInfo.Name = v
			setInfo.Type = c.getSetType(v, "SetMap")
			if setInfo.Type == pb.SetType_CIDRBLOCKS {
				populateCIDRBlockSet(setInfo)
			}
		} else {
			return fmt.Errorf("%w : %v", npmcommon.ErrSetNotExist, ipsetHashedName)
		}
	}

	if len(ipsetOrigin) > MinUnsortedIPSetLength {
		ruleRes.UnsortedIpset[ipsetHashedName] = ipsetOrigin
	}
	if strings.Contains(ipsetOrigin, "src") {
		ruleRes.SrcList = append(ruleRes.SrcList, setInfo)
	} else {
		ruleRes.DstList = append(ruleRes.DstList, setInfo)
	}
	return nil
}

// populate CIDRBlock set's content with ip addresses
func populateCIDRBlockSet(setInfo *pb.RuleResponse_SetInfo) {
	ipsetBuffer := bytes.NewBuffer(nil)
	cmdArgs := []string{"list", setInfo.HashedSetName}
	cmd := exec.Command(util.Ipset, cmdArgs...) //nolint:gosec

	cmd.Stdout = ipsetBuffer
	stderrBuffer := bytes.NewBuffer(nil)
	cmd.Stderr = stderrBuffer

	err := cmd.Run()
	if err != nil {
		_, err = stderrBuffer.WriteTo(ipsetBuffer)
		if err != nil {
			panic(err)
		}
	}
	curReadIndex := 0

	// finding the members field
	for curReadIndex < len(ipsetBuffer.Bytes()) {
		line, nextReadIndex := parse.Line(curReadIndex, ipsetBuffer.Bytes())
		curReadIndex = nextReadIndex
		if bytes.HasPrefix(line, MembersBytes) {
			break
		}
	}
	for curReadIndex < len(ipsetBuffer.Bytes()) {
		member, nextReadIndex := parse.Line(curReadIndex, ipsetBuffer.Bytes())
		setInfo.Contents = append(setInfo.Contents, string(member))
		curReadIndex = nextReadIndex
	}
}
