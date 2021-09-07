package policies

import (
	"fmt"
	"strings"

	"github.com/Microsoft/hcsshim/hcn"
)

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
	case Both:
		return ""
	}
	return ""
}

func getHCNProtocol(protocol string) string {
	// TODO need to check the protocol number of SCTP
	switch strings.ToLower(protocol) {
	case "tcp":
		return "6"
	case "udp":
		return "17"
	default:
		return ""
	}
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
