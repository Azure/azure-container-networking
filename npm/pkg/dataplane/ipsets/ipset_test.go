package ipsets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldBeInKernelAndCanDelete(t *testing.T) {
	s := &IPSetMetadata{"test-set", Namespace}
	l := &IPSetMetadata{"test-list", KeyLabelOfNamespace}
	tests := []struct {
		name          string
		set           *IPSet
		wantInKernel  bool
		wantDeletable bool
	}{
		{
			name: "only has selector reference",
			set: &IPSet{
				Name: s.GetPrefixName(),
				SetProperties: SetProperties{
					Type: s.Type,
					Kind: s.GetSetKind(),
				},
				SelectorReference: map[string]struct{}{
					"ref-1": {},
				},
			},
			wantInKernel:  true,
			wantDeletable: false,
		},
		{
			name: "only has netpol reference",
			set: &IPSet{
				Name: s.GetPrefixName(),
				SetProperties: SetProperties{
					Type: s.Type,
					Kind: s.GetSetKind(),
				},
				NetPolReference: map[string]struct{}{
					"ref-1": {},
				},
			},
			wantInKernel:  true,
			wantDeletable: false,
		},
		{
			name: "only referenced in list (in kernel)",
			set: &IPSet{
				Name: s.GetPrefixName(),
				SetProperties: SetProperties{
					Type: s.Type,
					Kind: s.GetSetKind(),
				},
				ipsetReferCount:  1,
				kernelReferCount: 1,
			},
			wantInKernel:  true,
			wantDeletable: false,
		},
		{
			name: "only referenced in list (not in kernel)",
			set: &IPSet{
				Name: s.GetPrefixName(),
				SetProperties: SetProperties{
					Type: s.Type,
					Kind: s.GetSetKind(),
				},
				ipsetReferCount: 1,
			},
			wantInKernel:  false,
			wantDeletable: false,
		},
		{
			name: "only has set members",
			set: &IPSet{
				Name: l.GetPrefixName(),
				SetProperties: SetProperties{
					Type: l.Type,
					Kind: l.GetSetKind(),
				},
				MemberIPSets: map[string]*IPSet{
					s.GetPrefixName(): NewIPSet(s),
				},
			},
			wantInKernel:  false,
			wantDeletable: false,
		},
		{
			name: "only has ip members",
			set: &IPSet{
				Name: s.GetPrefixName(),
				SetProperties: SetProperties{
					Type: s.Type,
					Kind: s.GetSetKind(),
				},
				IPPodKey: map[string]string{
					"1.2.3.4": "pod-a",
				},
			},
			wantInKernel:  false,
			wantDeletable: false,
		},
		{
			name: "deletable",
			set: &IPSet{
				Name: s.GetPrefixName(),
				SetProperties: SetProperties{
					Type: Namespace,
					Kind: s.GetSetKind(),
				},
			},
			wantInKernel:  false,
			wantDeletable: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantInKernel {
				require.True(t, tt.set.shouldBeInKernel())
			} else {
				require.False(t, tt.set.shouldBeInKernel())
			}

			if tt.wantDeletable {
				require.True(t, tt.set.canBeDeleted())
			} else {
				require.False(t, tt.set.canBeDeleted())
			}
		})
	}
}
