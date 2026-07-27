package policies

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
)

const (
	blockRulePriotity = 3000
	allowRulePriotity = 222
	policyIDPrefix    = "azure-acl"
)

var (
	protocolNumMap = map[Protocol]string{
		TCP:  "6",
		UDP:  "17",
		SCTP: "132",
		// HNS thinks 256 as ANY protocol
		UnspecifiedProtocol: "256",
	}

	ErrNamedPortsNotSupported     = errors.New("Named Port translation is not supported in windows dataplane")
	ErrNegativeMatchsNotSupported = errors.New("Negative match types is not supported in windows dataplane")
	ErrProtocolNotSupported       = errors.New("Protocol mentioned is not supported")
)

// aclPolicyID returns azure-acl-<network policy namespace>-<network policy name> format
// to differentiate ACLs among different network policies,
// but aclPolicy in the same network policy has the same aclPolicyID.
func aclPolicyID(policyNS, policyName string) string {
	return fmt.Sprintf("%s-%s-%s", policyIDPrefix, policyNS, policyName)
}

// NPMACLPolSettings is an adaption over the existing hcn.ACLPolicySettings
// default ACL settings does not contain ID field but HNS is happy with taking an ID
// this ID will help us woth correctly identifying the ACL policy when reading from HNS
type NPMACLPolSettings struct {
	// HNS is not happy with "ID"
	Id              string            `json:",omitempty"`
	Protocols       string            `json:",omitempty"` // EX: 6 (TCP), 17 (UDP), 1 (ICMPv4), 58 (ICMPv6), 2 (IGMP)
	Action          hcn.ActionType    `json:","`
	Direction       hcn.DirectionType `json:","`
	LocalAddresses  string            `json:",omitempty"`
	RemoteAddresses string            `json:",omitempty"`
	LocalPorts      string            `json:",omitempty"`
	RemotePorts     string            `json:",omitempty"`
	RuleType        hcn.RuleType      `json:",omitempty"`
	Priority        uint16            `json:",omitempty"`
}

func (orig NPMACLPolSettings) compare(newACL *NPMACLPolSettings) bool {
	return orig.Id == newACL.Id &&
		orig.Protocols == newACL.Protocols &&
		orig.Action == newACL.Action &&
		orig.Direction == newACL.Direction &&
		orig.LocalAddresses == newACL.LocalAddresses &&
		orig.RemoteAddresses == newACL.RemoteAddresses &&
		orig.LocalPorts == newACL.LocalPorts &&
		orig.RemotePorts == newACL.RemotePorts &&
		orig.RuleType == newACL.RuleType &&
		orig.Priority == newACL.Priority
}

func (acl *ACLPolicy) convertToAclSettings(aclID string) (*NPMACLPolSettings, error) {
	policySettings := &NPMACLPolSettings{}
	for _, setInfo := range acl.SrcList {
		if !setInfo.Included {
			return policySettings, ErrNegativeMatchsNotSupported
		}
	}

	if !acl.checkIPSets() {
		return policySettings, ErrNamedPortsNotSupported
	}

	policySettings.RuleType = hcn.RuleTypeSwitch
	policySettings.Id = aclID
	policySettings.Direction = getHCNDirection(acl.Direction)
	policySettings.Action = getHCNAction(acl.Target)

	// TODO need to have better priority handling
	policySettings.Priority = uint16(allowRulePriotity)
	if policySettings.Action == hcn.ActionTypeBlock {
		policySettings.Priority = uint16(blockRulePriotity)
	}
	protoNum, ok := protocolNumMap[acl.Protocol]
	if !ok {
		return policySettings, ErrProtocolNotSupported
	}

	if protoNum == "256" {
		policySettings.Protocols = ""
	} else {
		policySettings.Protocols = protoNum
	}
	// Ignore adding ruletype for now as there is a bug
	// policySettings.RuleType = hcn.RuleTypeSwitch

	// HNS has confusing Local and Remote address definitions
	// For Traffic Direction INGRESS
	// 	    LocalAddresses  = Source Sets
	// 	    RemoteAddresses = Destination Sets
	//      LocalPorts      = Destination Ports
	//      RemotePorts     = Source Ports

	// For Traffic Direction EGRESS
	// 	    LocalAddresses  = Source Sets
	// 	    RemoteAddresses = Destination Sets
	//      LocalPorts      = Source Ports
	//      RemotePorts     = Destination Ports

	// If we use IPs in ACLs, then INGRESS mapping is same, but EGRESS mapping will change to below
	// For Traffic Direction INGRESS
	// 		LocalAddresses  = Source IPs
	// 		RemoteAddresses = Destination IPs
	// For Traffic Direction EGRESS
	// 		LocalAddresses  = Destination IPs
	// 		RemoteAddresses = Source IPs

	var srcListStr, dstListStr string
	// if direct IPs are used, we leave local addresses to be an empty string
	if len(acl.SrcDirectIPs) > 0 || len(acl.DstDirectIPs) > 0 {
		srcListStr = strings.Join(acl.SrcDirectIPs, ",")
		dstListStr = strings.Join(acl.DstDirectIPs, ",")
		policySettings.LocalAddresses = ""
		if policySettings.Direction == hcn.DirectionTypeOut {
			// EGRESS: Remote = Destination IPs from policy
			policySettings.RemoteAddresses = dstListStr
		} else {
			// INGRESS: Remote = Source IPs from policy
			policySettings.RemoteAddresses = srcListStr
		}
	} else {
		// Original IPSet-based approach
		srcListStr = getAddrListFromSetInfo(acl.SrcList)
		dstListStr = getAddrListFromSetInfo(acl.DstList)
		policySettings.LocalAddresses = srcListStr
		policySettings.RemoteAddresses = dstListStr
	}

	dstPortStr := getPortStrFromPorts(acl.DstPorts)

	// Switch ports based on direction
	policySettings.RemotePorts = ""
	policySettings.LocalPorts = dstPortStr
	if policySettings.Direction == hcn.DirectionTypeOut {
		policySettings.LocalPorts = ""
		policySettings.RemotePorts = dstPortStr
	}

	return policySettings, nil
}

func (acl *ACLPolicy) checkIPSets() bool {
	for _, set := range acl.SrcList {
		if set.IPSet.Type == ipsets.NamedPorts {
			return false
		}

		if !set.hasKnownMatchType() {
			return false
		}
	}
	for _, set := range acl.DstList {
		if set.IPSet.Type == ipsets.NamedPorts {
			return false
		}

		if !set.hasKnownMatchType() {
			return false
		}
	}
	return true
}

func getAddrListFromSetInfo(setInfoList []SetInfo) string {
	setInfoStr := ""
	setInfoLen := len(setInfoList)
	for i, setInfo := range setInfoList {
		if i < setInfoLen-1 {
			setInfoStr += setInfo.IPSet.GetHashedName() + ","
		} else {
			setInfoStr += setInfo.IPSet.GetHashedName()
		}
	}
	return setInfoStr
}

func getPortStrFromPorts(port Ports) string {
	if port.Port == 0 {
		return ""
	}
	portStr := fmt.Sprintf("%d", port.Port)
	if port.EndPort != 0 {
		for tempPort := port.Port + 1; tempPort <= port.EndPort; tempPort++ {
			portStr += fmt.Sprintf(",%d", tempPort)
		}
	}
	return portStr
}

func getHCNDirection(direction Direction) hcn.DirectionType {
	switch direction {
	case Ingress:
		return hcn.DirectionTypeIn
	case Egress:
		return hcn.DirectionTypeOut
	}
	return ""
}

func getHCNAction(verdict Verdict) hcn.ActionType {
	switch verdict {
	case Allowed:
		return hcn.ActionTypeAllow
	case Dropped:
		return hcn.ActionTypeBlock
	}
	return ""
}
