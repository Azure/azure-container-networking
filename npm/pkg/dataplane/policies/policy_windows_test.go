package policies

import (
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/stretchr/testify/require"
)

func TestValidatePolicyDirectIPs(t *testing.T) {
	tests := []struct {
		name    string
		acl     *ACLPolicy
		wantErr bool
	}{
		{
			name: "valid ingress source CIDR",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Ingress,
				SrcDirectIPs: []string{"10.42.1.0/24"},
			},
		},
		{
			name: "valid egress destination CIDR",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Egress,
				DstDirectIPs: []string{"192.0.2.0/24"},
			},
		},
		{
			name: "direct IPs mixed with IPSet",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Ingress,
				SrcDirectIPs: []string{"10.42.1.0/24"},
				SrcList: []SetInfo{{
					IPSet:     ipsets.NewIPSetMetadata("cidr", ipsets.CIDRBlocks),
					Included:  true,
					MatchType: SrcMatch,
				}},
			},
			wantErr: true,
		},
		{
			name: "destination CIDR used for ingress",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Ingress,
				DstDirectIPs: []string{"10.42.1.0/24"},
			},
			wantErr: true,
		},
		{
			name: "source CIDR used for egress",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Egress,
				SrcDirectIPs: []string{"192.0.2.0/24"},
			},
			wantErr: true,
		},
		{
			name: "empty ingress source CIDR",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Ingress,
				SrcDirectIPs: []string{""},
			},
			wantErr: true,
		},
		{
			name: "IPv6 ingress source CIDR",
			acl: &ACLPolicy{
				Target:       Allowed,
				Direction:    Ingress,
				SrcDirectIPs: []string{"2001:db8::/64"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			networkPolicy := &NPMNetworkPolicy{
				PolicyKey: "victim/allow-cidrs",
				ACLs:      []*ACLPolicy{tt.acl},
			}
			NormalizePolicy(networkPolicy)
			err := ValidatePolicy(networkPolicy)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
