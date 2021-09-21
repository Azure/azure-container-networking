package policies

import (
	"fmt"

	"github.com/Microsoft/hcsshim/hcn"
)

var protocolNumMap = map[Protocol]string{
	TCP:  "6",
	UDP:  "17",
	ICMP: "1",
	SCTP: "132",
	// HNS thinks 256 as ANY protocol
	AnyProtocol: "256",
}

func convertToAclSettings(acl ACLPolicy) (hcn.AclPolicySetting, error) {
	policySettings := hcn.AclPolicySetting{}
	for _, setInfo := range acl.SrcList {
		if !setInfo.Included {
			return policySettings, fmt.Errorf("Windows Dataplane does not support negative matches. ACL: %+v", acl)
		}
	}

	return policySettings, nil
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
