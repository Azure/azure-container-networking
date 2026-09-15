package policies

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/stretchr/testify/require"
)

func TestWindowsACLRejectsNegativeSetsOnEitherSide(t *testing.T) {
	for _, direction := range []Direction{Ingress, Egress} {
		for _, destination := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/destination=%t", direction, destination), func(t *testing.T) {
				acl := NewACLPolicy(Allowed, direction)
				acl.Protocol = TCP
				negative := NewSetInfo("tenant", ipsets.KeyLabelOfNamespace, false, SrcMatch)
				if destination {
					acl.DstList = []SetInfo{negative}
				} else {
					acl.SrcList = []SetInfo{negative}
				}
				_, err := acl.convertToAclSettings("test-policy")
				require.ErrorIs(t, err, ErrNegativeMatchsNotSupported)
			})
		}
	}
}
