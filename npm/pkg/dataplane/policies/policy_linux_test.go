package policies

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePolicyRejectsDirectIPs(t *testing.T) {
	networkPolicy := &NPMNetworkPolicy{
		PolicyKey: "victim/allow-cidrs",
		ACLs: []*ACLPolicy{{
			Target:       Allowed,
			Direction:    Ingress,
			SrcDirectIPs: []string{"10.42.1.0/24"},
		}},
	}

	NormalizePolicy(networkPolicy)
	err := ValidatePolicy(networkPolicy)
	require.ErrorContains(t, err, "direct IP matching")
}
