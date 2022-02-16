package ipsets

import (
	"fmt"
	"os"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

type expectedInfo struct {
	mainCache        []setMembers
	toAddUpdateCache []*IPSetMetadata
	toDeleteCache    []string
	setsForKernel    []*IPSetMetadata
	// TODO add expected failure metric values too

	/*
		ipset metrics can be inferred from the above values:
		- num ipsets in cache/kernel
		- num entries (in kernel)
		- ipset inventory for kernel (num entries per set)
	*/
}

type setMembers struct {
	metadata *IPSetMetadata
	members  []member
}

type member struct {
	value string
	// either an IP/IP,PORT/CIDR or set name
	kind memberKind
}

type memberKind bool

const (
	isHashMember = memberKind(true)
	// TODO uncomment and use for list add/delete UTs
	// isSetMember = memberKind(false)

	testSetName  = "test-set"
	testListName = "test-list"
	testPodKey   = "test-pod-key"
	testPodIP    = "10.0.0.0"
)

var (
	applyOnNeedCfg = &IPSetManagerCfg{
		IPSetMode:   ApplyOnNeed,
		NetworkName: "azure",
	}

	applyAlwaysCfg = &IPSetManagerCfg{
		IPSetMode:   ApplyAllIPSets,
		NetworkName: "azure",
	}

	namespaceSet     = NewIPSetMetadata("test-set1", Namespace)
	keyLabelOfPodSet = NewIPSetMetadata("test-set2", KeyLabelOfPod)
	list             = NewIPSetMetadata("test-list1", KeyLabelOfNamespace)
)

// see ipsetmanager_linux_test.go for testing when an error occurs
func TestResetIPSets(t *testing.T) {
	metrics.ReinitializeAll()
	calls := GetResetTestCalls()
	ioShim := common.NewMockIOShim(calls)
	defer ioShim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)

	iMgr.CreateIPSets([]*IPSetMetadata{namespaceSet, keyLabelOfPodSet})

	metrics.IncNumIPSets()
	metrics.IncNumIPSets()
	metrics.AddEntryToIPSet("test1")
	metrics.AddEntryToIPSet("test1")
	metrics.AddEntryToIPSet("test2")

	require.NoError(t, iMgr.ResetIPSets())

	assertExpectedInfo(t, iMgr, &expectedInfo{
		mainCache:        nil,
		toAddUpdateCache: nil,
		toDeleteCache:    nil,
		setsForKernel:    nil,
	})
}

func TestCreateIPSet(t *testing.T) {
	type args struct {
		cfg       *IPSetManagerCfg
		metadatas []*IPSetMetadata
	}
	tests := []struct {
		name string
		args args
		expectedInfo
	}{
		{
			name: "Apply Always: create two new sets",
			args: args{
				cfg:       applyAlwaysCfg,
				metadatas: []*IPSetMetadata{namespaceSet, list},
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: nil},
					{metadata: list, members: nil},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet, list},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet, list},
			},
		},
		{
			name: "Apply On Need: create two new sets",
			args: args{
				cfg:       applyOnNeedCfg,
				metadatas: []*IPSetMetadata{namespaceSet, list},
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: nil},
					{metadata: list, members: nil},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
		},
		{
			name: "Apply Always: no-op for set that exists",
			args: args{
				cfg:       applyAlwaysCfg,
				metadatas: []*IPSetMetadata{namespaceSet, namespaceSet},
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: nil},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
		},
		{
			name: "Apply On Need: no-op for set that exists",
			args: args{
				cfg:       applyOnNeedCfg,
				metadatas: []*IPSetMetadata{namespaceSet, namespaceSet},
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: nil},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			ioShim := common.NewMockIOShim(nil)
			defer ioShim.VerifyCalls(t, nil)
			iMgr := NewIPSetManager(tt.args.cfg, ioShim)
			iMgr.CreateIPSets(tt.args.metadatas)
			assertExpectedInfo(t, iMgr, &tt.expectedInfo)
		})
	}
}

func TestDeleteIPSet(t *testing.T) {
	type args struct {
		cfg               *IPSetManagerCfg
		toCreateMetadatas []*IPSetMetadata
		toDeleteName      string
	}
	tests := []struct {
		name         string
		args         args
		expectedInfo expectedInfo
	}{
		{
			name: "Apply Always: delete set",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toDeleteName:      namespaceSet.GetPrefixName(),
			},
			expectedInfo: expectedInfo{
				mainCache:        nil,
				toAddUpdateCache: nil,
				toDeleteCache:    []string{namespaceSet.GetPrefixName()},
				setsForKernel:    nil,
			},
		},
		{
			name: "Apply On Need: delete set",
			args: args{
				cfg:               applyOnNeedCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toDeleteName:      namespaceSet.GetPrefixName(),
			},
			expectedInfo: expectedInfo{
				mainCache:        nil,
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
		},
		{
			name: "Apply Always: set doesn't exist",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toDeleteName:      keyLabelOfPodSet.GetPrefixName(),
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: nil},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
		},
		{
			name: "Apply On Need: set doesn't exist",
			args: args{
				cfg:               applyOnNeedCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toDeleteName:      keyLabelOfPodSet.GetPrefixName(),
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: nil},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			var calls []testutils.TestCmd
			if tt.args.cfg == applyAlwaysCfg {
				calls = GetApplyIPSetsTestCalls(tt.args.toCreateMetadatas, nil)
			}
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(tt.args.cfg, ioShim)
			iMgr.CreateIPSets(tt.args.toCreateMetadatas)
			require.NoError(t, iMgr.ApplyIPSets())
			iMgr.DeleteIPSet(tt.args.toDeleteName, util.SoftDelete)
			assertExpectedInfo(t, iMgr, &tt.expectedInfo)
		})
	}
}

func TestDeleteIPSetNotAllowed(t *testing.T) {
	// try to delete a list with a member and a set referenced in kernel (by a list)
	// must use applyAlwaysCfg for set to be in kernel
	// logic for ipset.canBeDeleted is tested elsewhere (in ipset_test.go)
	metrics.ReinitializeAll()
	calls := GetApplyIPSetsTestCalls([]*IPSetMetadata{list, namespaceSet}, nil)
	ioShim := common.NewMockIOShim(calls)
	defer ioShim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{list}, []*IPSetMetadata{namespaceSet}))
	require.NoError(t, iMgr.ApplyIPSets())

	iMgr.DeleteIPSet(namespaceSet.GetPrefixName(), util.SoftDelete)
	iMgr.DeleteIPSet(list.GetPrefixName(), util.SoftDelete)

	assertExpectedInfo(t, iMgr, &expectedInfo{
		mainCache: []setMembers{
			{metadata: list, members: nil},
			{metadata: namespaceSet, members: nil},
		},
		toAddUpdateCache: nil,
		toDeleteCache:    nil,
		setsForKernel:    nil,
	})
}

func TestAddToSets(t *testing.T) {
	// TODO test ip,port members, cidr members, and (if not done in controller) error throwing on invalid members
	ipv4 := "1.2.3.4"
	ipv6 := "2001:db8:0:0:0:0:2:1"

	type args struct {
		cfg                *IPSetManagerCfg
		toCreateMetadatas  []*IPSetMetadata
		toAddMetadatas     []*IPSetMetadata
		member             string
		memberExistedPrior bool
		hasDiffPodKey      bool
	}
	tests := []struct {
		name         string
		args         args
		expectedInfo expectedInfo
		wantErr      bool
	}{
		{
			name: "Apply Always: add new IP to 1 existing set and 1 non-existing set",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet, keyLabelOfPodSet},
				member:            ipv4,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{ipv4, isHashMember}}},
					{metadata: keyLabelOfPodSet, members: []member{{ipv4, isHashMember}}},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet, keyLabelOfPodSet},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet, keyLabelOfPodSet},
			},
			wantErr: false,
		},
		{
			name: "Apply On Need: add to new set",
			args: args{
				cfg:               applyOnNeedCfg,
				toCreateMetadatas: nil,
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            ipv4,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{ipv4, isHashMember}}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: false,
		},
		{
			name: "add IPv6",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            ipv6,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{ipv6, isHashMember}}},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
			wantErr: false,
		},
		{
			name: "add existing IP to set (same pod key)",
			args: args{
				cfg:                applyAlwaysCfg,
				toCreateMetadatas:  []*IPSetMetadata{namespaceSet},
				toAddMetadatas:     []*IPSetMetadata{namespaceSet},
				member:             ipv4,
				memberExistedPrior: true,
				hasDiffPodKey:      false,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{ipv4, isHashMember}}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
			wantErr: false,
		},
		{
			name: "add existing IP to set (diff pod key)",
			args: args{
				cfg:                applyAlwaysCfg,
				toCreateMetadatas:  []*IPSetMetadata{namespaceSet},
				toAddMetadatas:     []*IPSetMetadata{namespaceSet},
				member:             ipv4,
				memberExistedPrior: true,
				hasDiffPodKey:      false,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{ipv4, isHashMember}}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
			wantErr: false,
		},
		{
			name: "no-op for empty toAddMetadatas",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: nil,
				toAddMetadatas:    nil,
			},
			expectedInfo: expectedInfo{},
		},
		{
			// NOTE: we create the list and consider it dirty (because of the creation) even though an "add" error occurs
			name: "Apply Always: error on add to list",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: nil,
				toAddMetadatas:    []*IPSetMetadata{list},
				member:            ipv4,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: list, members: nil},
				},
				toAddUpdateCache: []*IPSetMetadata{list},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{list},
			},
			wantErr: true,
		},
		{
			// NOTE: we create the list even though an "add" error occurs
			name: "Apply On need: error on add to list",
			args: args{
				cfg:               applyOnNeedCfg,
				toCreateMetadatas: nil,
				toAddMetadatas:    []*IPSetMetadata{list},
				member:            ipv4,
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: list, members: nil},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			var calls []testutils.TestCmd
			if tt.args.cfg == applyAlwaysCfg {
				calls = GetApplyIPSetsTestCalls(tt.args.toCreateMetadatas, nil)
			}
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(tt.args.cfg, ioShim)
			iMgr.CreateIPSets(tt.args.toCreateMetadatas)

			podKey := "pod-a"
			otherPodKey := "pod-b"
			if tt.args.memberExistedPrior {
				require.NoError(t, iMgr.AddToSets(tt.args.toAddMetadatas, tt.args.member, podKey))
			}
			require.NoError(t, iMgr.ApplyIPSets())
			k := podKey
			if tt.args.hasDiffPodKey {
				k = otherPodKey
			}
			err := iMgr.AddToSets(tt.args.toAddMetadatas, tt.args.member, k)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assertExpectedInfo(t, iMgr, &tt.expectedInfo)
		})
	}
}

func TestAddToSetInKernelApplyOnNeed(t *testing.T) {
	tests := []struct {
		name      string
		metadata  *IPSetMetadata
		wantDirty bool
		wantErr   bool
	}{
		{
			name:      "success",
			metadata:  namespaceSet,
			wantDirty: true,
			wantErr:   false,
		},
		{
			name:      "failure",
			metadata:  list,
			wantDirty: false,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			podKey := "pod-a"
			ipv4 := "1.2.3.4"
			metadatas := []*IPSetMetadata{tt.metadata}

			metrics.ReinitializeAll()
			calls := GetApplyIPSetsTestCalls(metadatas, nil)
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyOnNeedCfg, ioShim)
			iMgr.CreateIPSets(metadatas)
			require.NoError(t, iMgr.AddReference(tt.metadata.GetPrefixName(), "ref", NetPolType))
			require.NoError(t, iMgr.ApplyIPSets())

			err := iMgr.AddToSets(metadatas, ipv4, podKey)
			var members []member
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				members = []member{{ipv4, isHashMember}}
			}
			var dirtySets []*IPSetMetadata
			if tt.wantDirty {
				dirtySets = []*IPSetMetadata{namespaceSet}
			}
			assertExpectedInfo(t, iMgr, &expectedInfo{
				mainCache: []setMembers{
					{metadata: tt.metadata, members: members},
				},
				toAddUpdateCache: dirtySets,
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{tt.metadata},
			})
		})
	}
}

func TestRemoveFromSets(t *testing.T) {
	calls := []testutils.TestCmd{}
	ioShim := common.NewMockIOShim(calls)
	defer ioShim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyOnNeedCfg, ioShim)

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})
	err := iMgr.AddToSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
	err = iMgr.RemoveFromSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestRemoveFromSetMissing(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	err := iMgr.RemoveFromSets([]*IPSetMetadata{setMetadata}, testPodIP, testPodKey)
	require.NoError(t, err)
}

func TestAddToListMissing(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata("testlabel", KeyLabelOfNamespace)
	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)
}

func TestAddToList(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata, listMetadata})

	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set := iMgr.GetIPSet(listMetadata.GetPrefixName())
	require.NotNil(t, set)
	require.Equal(t, listMetadata.GetPrefixName(), set.Name)
	require.Equal(t, util.GetHashedName(listMetadata.GetPrefixName()), set.HashedName)
	require.Equal(t, 1, len(set.MemberIPSets))
	require.Equal(t, setMetadata.GetPrefixName(), set.MemberIPSets[setMetadata.GetPrefixName()].Name)
}

func TestRemoveFromList(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata, listMetadata})

	err := iMgr.AddToLists([]*IPSetMetadata{listMetadata}, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set := iMgr.GetIPSet(listMetadata.GetPrefixName())
	require.NotNil(t, set)
	require.Equal(t, listMetadata.GetPrefixName(), set.Name)
	require.Equal(t, util.GetHashedName(listMetadata.GetPrefixName()), set.HashedName)
	require.Equal(t, 1, len(set.MemberIPSets))
	require.Equal(t, setMetadata.GetPrefixName(), set.MemberIPSets[setMetadata.GetPrefixName()].Name)

	err = iMgr.RemoveFromList(listMetadata, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)

	set = iMgr.GetIPSet(listMetadata.GetPrefixName())
	require.NotNil(t, set)
	require.Equal(t, 0, len(set.MemberIPSets))
}

func TestRemoveFromListMissing(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	listMetadata := NewIPSetMetadata(testListName, KeyLabelOfNamespace)
	iMgr.CreateIPSets([]*IPSetMetadata{listMetadata})

	err := iMgr.RemoveFromList(listMetadata, []*IPSetMetadata{setMetadata})
	require.NoError(t, err)
}
func TestGetIPsFromSelectorIPSets(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setsTocreate := []*IPSetMetadata{
		{
			Name: "setNs1",
			Type: Namespace,
		},
		{
			Name: "setpod1",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod2",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod3",
			Type: KeyValueLabelOfPod,
		},
	}

	iMgr.CreateIPSets(setsTocreate)

	err := iMgr.AddToSets(setsTocreate, "10.0.0.1", "test")
	require.NoError(t, err)

	err = iMgr.AddToSets(setsTocreate, "10.0.0.2", "test1")
	require.NoError(t, err)

	err = iMgr.AddToSets([]*IPSetMetadata{setsTocreate[0], setsTocreate[2], setsTocreate[3]}, "10.0.0.3", "test3")
	require.NoError(t, err)

	ipsetList := map[string]struct{}{}
	for _, v := range setsTocreate {
		ipsetList[v.GetPrefixName()] = struct{}{}
	}
	ips, err := iMgr.GetIPsFromSelectorIPSets(ipsetList)
	require.NoError(t, err)

	require.Equal(t, 2, len(ips))

	expectedintersection := map[string]struct{}{
		"10.0.0.1": {},
		"10.0.0.2": {},
	}

	require.Equal(t, ips, expectedintersection)
}

func TestAddDeleteSelectorReferences(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setsTocreate := []*IPSetMetadata{
		{
			Name: "setNs1",
			Type: Namespace,
		},
		{
			Name: "setpod1",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod2",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod3",
			Type: NestedLabelOfPod,
		},
		{
			Name: "setpod4",
			Type: KeyLabelOfPod,
		},
	}
	networkPolicName := "testNetworkPolicy"

	for _, k := range setsTocreate {
		err := iMgr.AddReference(k.GetPrefixName(), networkPolicName, SelectorType)
		require.Error(t, err)
	}

	iMgr.CreateIPSets(setsTocreate)

	// Add setpod4 to setpod3
	err := iMgr.AddToLists([]*IPSetMetadata{setsTocreate[3]}, []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		err = iMgr.AddReference(v.GetPrefixName(), networkPolicName, SelectorType)
		require.NoError(t, err)
	}

	require.Equal(t, 5, len(iMgr.toAddOrUpdateCache))
	require.Equal(t, 0, len(iMgr.toDeleteCache))

	for _, v := range setsTocreate {
		err = iMgr.DeleteReference(v.GetPrefixName(), networkPolicName, SelectorType)
		if err != nil {
			t.Errorf("DeleteReference failed with error %s", err.Error())
		}
	}

	require.Equal(t, 0, len(iMgr.toAddOrUpdateCache))
	require.Equal(t, 5, len(iMgr.toDeleteCache))

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName(), util.SoftDelete)
	}

	// Above delete will not remove setpod3 and setpod4
	// because they are referencing each other
	require.Equal(t, 2, len(iMgr.setMap))

	err = iMgr.RemoveFromList(setsTocreate[3], []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName(), util.SoftDelete)
	}

	for _, v := range setsTocreate {
		set := iMgr.GetIPSet(v.GetPrefixName())
		require.Nil(t, set)
	}
}

func TestAddDeleteNetPolReferences(t *testing.T) {
	iMgr := NewIPSetManager(applyOnNeedCfg, common.NewMockIOShim([]testutils.TestCmd{}))
	setsTocreate := []*IPSetMetadata{
		{
			Name: "setNs1",
			Type: Namespace,
		},
		{
			Name: "setpod1",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod2",
			Type: KeyLabelOfPod,
		},
		{
			Name: "setpod3",
			Type: NestedLabelOfPod,
		},
		{
			Name: "setpod4",
			Type: KeyLabelOfPod,
		},
	}
	networkPolicName := "testNetworkPolicy"

	iMgr.CreateIPSets(setsTocreate)
	err := iMgr.AddToLists([]*IPSetMetadata{setsTocreate[3]}, []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		err = iMgr.AddReference(v.GetPrefixName(), networkPolicName, NetPolType)
		require.NoError(t, err)
	}

	require.Equal(t, 5, len(iMgr.toAddOrUpdateCache))
	require.Equal(t, 0, len(iMgr.toDeleteCache))
	for _, v := range setsTocreate {
		err = iMgr.DeleteReference(v.GetPrefixName(), networkPolicName, NetPolType)
		require.NoError(t, err)
	}

	require.Equal(t, 0, len(iMgr.toAddOrUpdateCache))
	require.Equal(t, 5, len(iMgr.toDeleteCache))

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName(), util.SoftDelete)
	}

	// Above delete will not remove setpod3 and setpod4
	// because they are referencing each other
	require.Equal(t, 2, len(iMgr.setMap))

	err = iMgr.RemoveFromList(setsTocreate[3], []*IPSetMetadata{setsTocreate[4]})
	require.NoError(t, err)

	for _, v := range setsTocreate {
		iMgr.DeleteIPSet(v.GetPrefixName(), util.SoftDelete)
	}

	for _, v := range setsTocreate {
		set := iMgr.GetIPSet(v.GetPrefixName())
		require.Nil(t, set)
	}

	for _, v := range setsTocreate {
		err = iMgr.DeleteReference(v.GetPrefixName(), networkPolicName, NetPolType)
		require.Error(t, err)
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}

func TestValidateIPBlock(t *testing.T) {
	tests := []struct {
		name    string
		ipblock string
		wantErr bool
	}{
		{
			name:    "cidr",
			ipblock: "172.17.0.0/16",
			wantErr: false,
		},
		{
			name:    "except ipblock",
			ipblock: "172.17.1.0/24 nomatch",
			wantErr: false,
		},
		{
			name:    "incorrect ip format",
			ipblock: "1234",
			wantErr: true,
		},
		{
			name:    "incorrect ip range",
			ipblock: "256.1.2.3",
			wantErr: true,
		},
		{
			name:    "empty cidr",
			ipblock: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPBlock(tt.ipblock)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func assertExpectedInfo(t *testing.T, iMgr *IPSetManager, info *expectedInfo) {
	// 1. assert cache contents
	require.Equal(t, len(info.mainCache), len(iMgr.setMap), "main cache size mismatch")
	for _, setMembers := range info.mainCache {
		setName := setMembers.metadata.GetPrefixName()
		require.True(t, iMgr.exists(setName), "set %s not found in main cache", setName)
		set := iMgr.GetIPSet(setName)
		require.NotNil(t, set, "set %s should be non-nil", setName)
		require.Equal(t, util.GetHashedName(setName), set.HashedName, "HashedName mismatch")
		for _, member := range setMembers.members {
			set := iMgr.setMap[setName]
			if member.kind == isHashMember {
				_, ok := set.IPPodKey[member.value]
				require.True(t, ok, "ip member %s not found in set %s", member.value, setName)
			} else {
				_, ok := set.MemberIPSets[member.value]
				require.True(t, ok, "set member %s not found in list %s", member.value, setName)
			}
		}
	}

	require.Equal(t, len(info.toAddUpdateCache), len(iMgr.toAddOrUpdateCache), "toAddUpdateCache size mismatch")
	for _, setMetadata := range info.toAddUpdateCache {
		setName := setMetadata.GetPrefixName()
		_, ok := iMgr.toAddOrUpdateCache[setName]
		require.True(t, ok, "set %s not in the toAddUpdateCache")
		require.True(t, iMgr.exists(setName), "set %s not in the main cache but is in the toAddUpdateCache", setName)
	}

	require.Equal(t, len(info.toDeleteCache), len(iMgr.toDeleteCache), "toDeleteCache size mismatch")
	for _, setName := range info.toDeleteCache {
		_, ok := iMgr.toDeleteCache[setName]
		require.True(t, ok, "set %s not found in toDeleteCache", setName)
	}

	for _, setMetadata := range info.setsForKernel {
		setName := setMetadata.GetPrefixName()
		require.True(t, iMgr.exists(setName), "kernel set %s not found", setName)
		set := iMgr.setMap[setName]
		require.True(t, iMgr.shouldBeInKernel(set), "set %s should be in kernel", setName)
	}

	// 2. assert prometheus metrics
	// at this point, the expected cache/kernel is the same as the actual cache/kernel
	numIPSetsInKernel, err := metrics.GetNumIPSets()
	promutil.NotifyIfErrors(t, err)
	fmt.Println(numIPSetsInKernel)
	// TODO uncomment and remove print statement when we have prometheus metric for in kernel
	// require.Equal(t, len(info.setsInKernel), numIPSetsInKernel, "numIPSetsInKernel mismatch")

	// TODO update get function when we have prometheus metric for in kernel
	numIPSetsInCache, err := metrics.GetNumIPSets()
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, len(iMgr.setMap), numIPSetsInCache, "numIPSetsInCache mismatch")

	// the setMap is equal
	expectedNumEntriesInCache := 0
	expectedNumEntriesInKernel := 0
	for _, set := range iMgr.setMap {
		// one of IPPodKey or MemberIPSets should be nil
		expectedNumEntriesInCache += len(set.IPPodKey) + len(set.MemberIPSets)
		if iMgr.shouldBeInKernel(set) {
			expectedNumEntriesInKernel += len(set.IPPodKey) + len(set.MemberIPSets)
		}
	}

	numEntriesInKernel, err := metrics.GetNumIPSetEntries()
	promutil.NotifyIfErrors(t, err)
	fmt.Println(numEntriesInKernel)
	// TODO uncomment and remove print statement when we have prometheus metric for in kernel
	// require.Equal(t, expectedNumEntriesInKernel, numEntriesInKernel, "numEntriesInKernel mismatch")

	// TODO update get func when we have prometheus metric for in kernel
	numEntriesInCache, err := metrics.GetNumIPSetEntries()
	promutil.NotifyIfErrors(t, err)
	require.Equal(t, expectedNumEntriesInCache, numEntriesInCache)
	for _, set := range iMgr.setMap {
		expectedNumEntries := 0
		// TODO replace bool with iMgr.shouldBeInKernel(set) when we have prometheus metric for in kernel
		if set.Name != "pizza" {
			// one of IPPodKey or MemberIPSets should be nil
			expectedNumEntries = len(set.IPPodKey) + len(set.MemberIPSets)
		}
		numEntries, err := metrics.GetNumEntriesForIPSet(set.Name)
		promutil.NotifyIfErrors(t, err)
		require.Equal(t, expectedNumEntries, numEntries, "numEntries mismatch for set %s", set.Name)
	}
}
