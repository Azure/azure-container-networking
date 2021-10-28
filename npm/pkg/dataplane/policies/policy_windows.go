package policies

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Microsoft/hcsshim/hcn"
)

var (
	protocolNumMap = map[Protocol]string{
		TCP:  "6",
		UDP:  "17",
		ICMP: "1",
		SCTP: "132",
		// HNS thinks 256 as ANY protocol
		AnyProtocol: "256",
	}

	ErrNamedPortsNotSupported     = errors.New("Named Port translation is not supported in windows dataplane")
	ErrNegativeMatchsNotSupported = errors.New("Negative match types is not supported in windows dataplane")
	ErrProtocolNotSupported       = errors.New("Protocol mentioned is not supported")
)

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

func (orig NPMACLPolSettings) compare(new *NPMACLPolSettings) bool {
	return orig.Id == new.Id &&
		orig.Protocols == new.Protocols &&
		orig.Action == new.Action &&
		orig.Direction == new.Direction &&
		orig.LocalAddresses == new.LocalAddresses &&
		orig.RemoteAddresses == new.RemoteAddresses &&
		orig.LocalPorts == new.LocalPorts &&
		orig.RemotePorts == new.RemotePorts &&
		orig.RuleType == new.RuleType &&
		orig.Priority == new.Priority
}

func (acl *ACLPolicy) convertToAclSettings() (*NPMACLPolSettings, error) {
	policySettings := &NPMACLPolSettings{}
	for _, setInfo := range acl.SrcList {
		if !setInfo.Included {
			return policySettings, ErrNegativeMatchsNotSupported
		}
	}

	if !acl.checkIPSets() {
		return policySettings, ErrNamedPortsNotSupported
	}

	policySettings.Id = acl.PolicyID
	policySettings.Direction = getHCNDirection(acl.Direction)
	policySettings.Action = getHCNAction(acl.Target)

	// TODO need to have better priority handling
	policySettings.Priority = uint16(222)
	if policySettings.Action == hcn.ActionTypeBlock {
		policySettings.Priority = uint16(3000)
	}
	if acl.Protocol == "" {
		acl.Protocol = AnyProtocol
	}
	protoNum, ok := protocolNumMap[acl.Protocol]
	if !ok {
		return policySettings, ErrProtocolNotSupported
	}
	policySettings.Protocols = protoNum

	// Ignore adding ruletype for now as there is a bug
	// policySettings.RuleType = hcn.RuleTypeSwitch

	// ACLPolicy settings uses ID field of SetPolicy in LocalAddresses or RemoteAddresses
	srcListStr := getAddrListFromSetInfo(acl.SrcList)
	srcPortStr := getPortStrFromPorts(acl.SrcPorts)
	dstListStr := getAddrListFromSetInfo(acl.DstList)
	dstPortStr := getPortStrFromPorts(acl.DstPorts)

	// HNS has confusing Local and Remote address defintions
	// For Traffic Direction INGRESS
	// 		LocalAddresses = Source IPs
	// 		RemoteAddresses = Destination IPs
	// For Traffic Direction EGRESS
	// 		LocalAddresses = Destination IPs
	// 		RemoteAddresses = Source IPs
	policySettings.LocalAddresses = srcListStr
	policySettings.LocalPorts = srcPortStr
	policySettings.RemoteAddresses = dstListStr
	policySettings.RemotePorts = dstPortStr
	if policySettings.Direction == hcn.DirectionTypeOut {
		policySettings.LocalAddresses = dstListStr
		policySettings.LocalPorts = dstPortStr
		policySettings.RemoteAddresses = srcListStr
		policySettings.RemotePorts = srcPortStr
	}

	return policySettings, nil
}

func (acl *ACLPolicy) checkIPSets() bool {
	for _, set := range acl.SrcList {
		if set.IPSet.Type == ipsets.NamedPorts {
			return false
		}

		if set.MatchType != "src" && set.MatchType != "dst" {
			return false
		}
	}
	for _, set := range acl.DstList {
		if set.IPSet.Type == ipsets.NamedPorts {
			return false
		}

		if set.MatchType != "src" && set.MatchType != "dst" {
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

func getPortStrFromPorts(ports []Ports) string {
	portStr := ""
	for i, port := range ports {
		if port.Port == 0 {
			continue
		}
		tempPortStr := fmt.Sprintf("%d", port.Port)
		if port.EndPort != 0 {
			for tempPort := port.Port + 1; tempPort <= port.EndPort; tempPort++ {
				tempPortStr += fmt.Sprintf(",%d", tempPort)
			}
		}
		if i < len(ports)-1 {
			portStr += tempPortStr + ","
		} else {
			portStr += tempPortStr
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
