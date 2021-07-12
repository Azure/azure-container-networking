package converter

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"

	"github.com/Azure/azure-container-networking/hack/dataplaneParser/iptable"
	"github.com/Azure/azure-container-networking/hack/dataplaneParser/parser"
)

type Converter struct {
}

// JSON
// type RuleResponse struct {
// 	Chain         string            `json:"chain"`
// 	SrcList       []string          `json:"srcList"`
// 	DstList       []string          `json:"dstList"`
// 	Protocol      string            `json:"protocol"`
// 	DPort         string            `json:"dport"`
// 	SPort         string            `json:"sport"`
// 	Allowed       bool              `json:"allowed"`
// 	Direction     iptable.Direction `json:"direction"`
// 	UnsortedIpset map[string]string `json:"unsortedIpset"` // key: ipset name, value: src,dst or dst,dst
// }

const (
	EGRESS  iptable.Direction = "EGRESS"
	INGRESS iptable.Direction = "INGRESS"
)

// ConvertIptablesObject returns a JSON object of an iptable go oject
func (c *Converter) ConvertIptablesObject(iptableObj *iptable.Iptables) []byte {
	// iptableJson, err := json.Marshal(iptableObj)
	iptableJson, err := json.MarshalIndent(iptableObj, "", "    ") // pretty print
	if err != nil {
		log.Fatalf("Error occured during marshaling. Error: %s", err.Error())
	}
	return iptableJson
}

// GetRulesFromIptable returns a list of JSON rule object of an iptable
func (c *Converter) GetRulesFromIptable(tableName string, iptableBuffer *bytes.Buffer) *RuleResponseList {
	p := &parser.Parser{}
	ipTableObj := p.ParseIptablesObject(tableName, iptableBuffer)
	ruleResList := &RuleResponseList{}
	for _, v := range ipTableObj.Chains() {
		chainRules := c.getRulesFromChain(v)
		ruleResList.RuleList = append(ruleResList.RuleList, chainRules...)
	}
	return ruleResList
}

func (c *Converter) getRulesFromChain(iptableChainObj *iptable.IptablesChain) []*RuleResponse {
	rules := make([]*RuleResponse, 0)
	for _, v := range iptableChainObj.Rules() {
		rule := &RuleResponse{}
		rule.Chain = iptableChainObj.Name()
		rule.Protocol = v.Protocol()
		switch v.Target().Name() {
		case "MARK":
			rule.Allowed = true
		case "DROP":
			rule.Allowed = false
		default:
			// ignore other targets
			continue
		}
		direction := c.getRuleDirection(iptableChainObj.Name())
		if direction != "" {
			rule.Direction = string(direction)
		}

		c.getModulesFromRule(v.Modules(), rule)
		rules = append(rules, rule)

	}
	return rules
}

func (c *Converter) getRuleDirection(iptableChainObjName string) iptable.Direction {
	if strings.Contains(iptableChainObjName, "EGRESS") {
		return EGRESS
	} else if strings.Contains(iptableChainObjName, "INGRESS") {
		return INGRESS
	} else {
		return ""
	}
}

func (c *Converter) getModulesFromRule(m_list []*iptable.Module, ruleRes *RuleResponse) {
	ruleRes.SrcList = make([]string, 0)
	ruleRes.DstList = make([]string, 0)
	ruleRes.UnsortedIpset = make(map[string]string)
	for _, m := range m_list {
		switch m.Verb() {
		case "set":
			//set module
			OptionValueMap := m.OptionValueMap()
			ipsetName := OptionValueMap["match-set"][0]
			ipsetOrigin := OptionValueMap["match-set"][1]
			if len(ipsetOrigin) > 3 {
				ruleRes.UnsortedIpset[ipsetName] = ipsetOrigin
			}
			if strings.Contains(ipsetOrigin, "src") {
				ruleRes.SrcList = append(ruleRes.SrcList, ipsetName)
			} else {
				ruleRes.DstList = append(ruleRes.DstList, ipsetName)
			}
		case "tcp":
			// tcp module TODO: other protocol
			OptionValueMap := m.OptionValueMap
			for k, v := range OptionValueMap() {
				if k == "dport" {
					ruleRes.DPort = v[0]
				} else {
					ruleRes.SPort = v[0]
				}
			}
		default:
			continue
		}
	}
}
