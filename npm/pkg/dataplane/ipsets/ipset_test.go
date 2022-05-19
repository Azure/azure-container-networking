package ipsets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAffiliatedIPs(t *testing.T) {
	s1 := NewIPSet(NewIPSetMetadata("test-set1", Namespace))
	s1.IPPodKey["1.1.1.1"] = "pod-w"
	s1.IPPodKey["2.2.2.2"] = "pod-x"
	s2 := NewIPSet(NewIPSetMetadata("test-set2", Namespace))
	s2.IPPodKey["3.3.3.3"] = "pod-y"
	s2.IPPodKey["4.4.4.4"] = "pod-z"

	// 1 IP from each set above
	s3 := NewIPSet(NewIPSetMetadata("test-set3", Namespace))
	s3.IPPodKey["1.1.1.1"] = "pod-w"
	s3.IPPodKey["4.4.4.4"] = "pod-z"

	l := NewIPSet(NewIPSetMetadata("test-list", KeyLabelOfNamespace))
	l.MemberIPSets[s1.Name] = s1
	l.MemberIPSets[s2.Name] = s2

	expected := map[string]struct{}{
		"1.1.1.1": {},
		"2.2.2.2": {},
	}
	require.Equal(t, expected, s1.affiliatedIPs(), "unexpected affiliated IPs for set 1")

	expected = map[string]struct{}{
		"1.1.1.1": {},
		"2.2.2.2": {},
		"3.3.3.3": {},
		"4.4.4.4": {},
	}
	require.Equal(t, expected, l.affiliatedIPs(), "unexpected affiliated IPs for list")

	intersection := s3.affiliatedIPs()
	l.intersectAffiliatedIPs(intersection)
	expected = map[string]struct{}{
		"1.1.1.1": {},
		"4.4.4.4": {},
	}
	require.Equal(t, expected, intersection, "unexpected intersection (direction #1)")

	intersection = l.affiliatedIPs()
	s3.intersectAffiliatedIPs(intersection)
	expected = map[string]struct{}{
		"1.1.1.1": {},
		"4.4.4.4": {},
	}
	require.Equal(t, expected, intersection, "unexpected intersection (direction #2)")
}

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
