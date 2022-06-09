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
	// setsForKernel represents the sets in toAddUpdateCache that should be in the kernel
	setsForKernel []*IPSetMetadata

	/*
		ipset metrics can be inferred from the above values:
		- num ipsets in cache/kernel
		- num entries (in kernel)
		- ipset inventory for kernel (num entries per set)
	*/
}

type setMembers struct {
	metadata           *IPSetMetadata
	members            []member
	selectorReferences []string
	netPolReferences   []string
}

type member struct {
	value string
	// either an IP/IP,PORT/CIDR or set name
	kind memberKind
}

type memberKind bool

const (
	isHashMember = memberKind(true)
	isSetMember  = memberKind(false)

	testSetName   = "test-set"
	testListName  = "test-list"
	testPodKey    = "test-pod-key"
	testPodIP     = "10.0.0.0"
	testNetPolKey = "test-ns/test-netpol"
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
	portSet          = NewIPSetMetadata("test-set3", NamedPorts)
	list             = NewIPSetMetadata("test-list1", KeyLabelOfNamespace)
)

func TestReconcileCache(t *testing.T) {
	type args struct {
		cfg          *IPSetManagerCfg
		setsInKernel []*IPSetMetadata
	}
	deletableSet := keyLabelOfPodSet
	otherSet := namespaceSet
	bothMetadatas := []*IPSetMetadata{deletableSet, otherSet}
	tests := []struct {
		name          string
		args          args
		toDeleteCache []string
	}{
		{
			name:          "Apply Always",
			args:          args{cfg: applyAlwaysCfg, setsInKernel: bothMetadatas},
			toDeleteCache: []string{deletableSet.GetPrefixName()},
		},
		{
			name:          "Apply On Need",
			args:          args{cfg: applyOnNeedCfg, setsInKernel: nil},
			toDeleteCache: nil,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			calls := GetApplyIPSetsTestCalls(tt.args.setsInKernel, nil)
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(tt.args.cfg, ioShim)

			// create two sets, one which can be deleted
			iMgr.CreateIPSets(bothMetadatas)
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{otherSet}, testPodIP, testPodKey))
			require.NoError(t, iMgr.ApplyIPSets())

			iMgr.Reconcile()
			assertExpectedInfo(t, iMgr, &expectedInfo{
				mainCache: []setMembers{
					{metadata: otherSet, members: []member{{testPodIP, isHashMember}}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    tt.toDeleteCache,
				setsForKernel:    nil,
			})
		})
	}
}

// only care about ApplyAllIPSets mode since ApplyOnNeed mode doesn't update the toDeleteCache
func TestReconcileAndLaterDelete(t *testing.T) {
	deletableSet := keyLabelOfPodSet
	otherSet := namespaceSet
	thirdSet := list
	tests := []struct {
		name              string
		setsToAdd         []*IPSetMetadata
		shouldDeleteLater bool
		*expectedInfo
	}{
		{
			name:              "apply the delete only",
			setsToAdd:         nil,
			shouldDeleteLater: true,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: otherSet, members: []member{{testPodIP, isHashMember}}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    []string{deletableSet.GetPrefixName()},
			},
		},
		{
			name:              "delete the set and add a different one",
			setsToAdd:         []*IPSetMetadata{thirdSet},
			shouldDeleteLater: true,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: otherSet, members: []member{{testPodIP, isHashMember}}},
					{metadata: thirdSet},
				},
				toAddUpdateCache: []*IPSetMetadata{thirdSet},
				toDeleteCache:    []string{deletableSet.GetPrefixName()},
			},
		},
		{
			name:              "add the set back",
			setsToAdd:         []*IPSetMetadata{deletableSet},
			shouldDeleteLater: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: deletableSet},
					{metadata: otherSet, members: []member{{testPodIP, isHashMember}}},
				},
				toAddUpdateCache: []*IPSetMetadata{deletableSet},
				toDeleteCache:    nil,
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			originalMetadatas := []*IPSetMetadata{deletableSet, otherSet}
			var toDeleteMetadatas []*IPSetMetadata
			if tt.shouldDeleteLater {
				toDeleteMetadatas = []*IPSetMetadata{deletableSet}
			}
			calls := GetApplyIPSetsTestCalls(originalMetadatas, nil)
			calls = append(calls, GetApplyIPSetsTestCalls(tt.setsToAdd, toDeleteMetadatas)...)
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)
			// create two sets, one which can be deleted
			iMgr.CreateIPSets(originalMetadatas)
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{namespaceSet}, testPodIP, testPodKey))
			require.NoError(t, iMgr.ApplyIPSets())
			iMgr.Reconcile()

			iMgr.CreateIPSets(tt.setsToAdd)
			assertExpectedInfo(t, iMgr, tt.expectedInfo)
			require.NoError(t, iMgr.ApplyIPSets())
		})
	}
}

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
					{metadata: namespaceSet},
					{metadata: list},
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
					{metadata: namespaceSet},
					{metadata: list},
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
					{metadata: namespaceSet},
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
					{metadata: namespaceSet},
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
					{metadata: namespaceSet},
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
					{metadata: namespaceSet},
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
			if tt.args.cfg.IPSetMode == ApplyAllIPSets {
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

func TestDeleteAfterCreate(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{}
	ioShim := common.NewMockIOShim(calls)
	defer ioShim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyOnNeedCfg, ioShim)

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})
	iMgr.DeleteIPSet(setMetadata.GetPrefixName(), util.SoftDelete)
	assertExpectedInfo(t, iMgr, &expectedInfo{})
}

func TestCreateAfterHardDelete(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{}
	ioShim := common.NewMockIOShim(calls)
	defer ioShim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)

	setMetadata := NewIPSetMetadata(testSetName, Namespace)
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{setMetadata}, "1.2.3.4", "pod-a"))
	// clear dirty cache, otherwise a set deletion will be a no-op
	iMgr.clearDirtyCache()

	numIPSetsInCache, _ := metrics.GetNumIPSets()
	fmt.Println(numIPSetsInCache)

	iMgr.DeleteIPSet(setMetadata.GetPrefixName(), util.ForceDelete)
	numIPSetsInCache, _ = metrics.GetNumIPSets()
	fmt.Println(numIPSetsInCache)

	assertExpectedInfo(t, iMgr, &expectedInfo{
		toDeleteCache: []string{setMetadata.GetPrefixName()},
	})

	iMgr.CreateIPSets([]*IPSetMetadata{setMetadata})
	assertExpectedInfo(t, iMgr, &expectedInfo{
		mainCache: []setMembers{
			{
				metadata: setMetadata,
			},
		},
		toAddUpdateCache: []*IPSetMetadata{setMetadata},
	})
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
			{metadata: list, members: []member{{namespaceSet.GetPrefixName(), isSetMember}}},
			{metadata: namespaceSet},
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
			name: "Apply On Need: cidr add to new set",
			args: args{
				cfg:               applyOnNeedCfg,
				toCreateMetadatas: nil,
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "10.0.0.0/8",
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{"10.0.0.0/8", isHashMember}}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: false,
		},
		{
			name: "Apply On Need: bad cidr add to new set",
			args: args{
				cfg:               applyOnNeedCfg,
				toCreateMetadatas: nil,
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "310.0.0.0/8",
			},
			expectedInfo: expectedInfo{
				mainCache:        []setMembers{},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
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
					{metadata: namespaceSet, members: []member{}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add cidr",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "10.0.0.0/8",
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{{"10.0.0.0/8", isHashMember}}},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
			wantErr: false,
		},
		{
			name: "add bad cidr",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "310.0.0.0/8",
			},
			expectedInfo: expectedInfo{
				mainCache:        []setMembers{{metadata: namespaceSet, members: []member{}}},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add bad cidr 2",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "x.x.x.x/8",
			},
			expectedInfo: expectedInfo{
				mainCache:        []setMembers{{metadata: namespaceSet, members: []member{}}},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add bad ip 1",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "x.x.x.x",
			},
			expectedInfo: expectedInfo{
				mainCache:        []setMembers{{metadata: namespaceSet, members: []member{}}},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add bad ip port 1",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "x.x.x.x,80",
			},
			expectedInfo: expectedInfo{
				mainCache:        []setMembers{{metadata: namespaceSet, members: []member{}}},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
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
				setsForKernel:    nil,
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
				setsForKernel:    nil,
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
					{metadata: list},
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
					{metadata: list},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add empty ip",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            "",
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add empty ip with port",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{namespaceSet},
				toAddMetadatas:    []*IPSetMetadata{namespaceSet},
				member:            ",80",
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, members: []member{}},
				},
				toAddUpdateCache: nil,
				toDeleteCache:    nil,
				setsForKernel:    nil,
			},
			wantErr: true,
		},
		{
			name: "add ipv4 with port",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{portSet},
				toAddMetadatas:    []*IPSetMetadata{portSet},
				member:            "1.1.1.1,80",
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: portSet, members: []member{{"1.1.1.1,80", isHashMember}}},
				},
				toAddUpdateCache: []*IPSetMetadata{portSet},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{portSet},
			},
			wantErr: false,
		},
		{
			name: "add cidr with nomatch",
			args: args{
				cfg:               applyAlwaysCfg,
				toCreateMetadatas: []*IPSetMetadata{portSet},
				toAddMetadatas:    []*IPSetMetadata{portSet},
				member:            "10.10.2.0/28 nomatch",
			},
			expectedInfo: expectedInfo{
				mainCache: []setMembers{
					{metadata: portSet, members: []member{{"10.10.2.0/28 nomatch", isHashMember}}},
				},
				toAddUpdateCache: []*IPSetMetadata{portSet},
				toDeleteCache:    nil,
				setsForKernel:    []*IPSetMetadata{portSet},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			var calls []testutils.TestCmd
			if tt.args.cfg.IPSetMode == ApplyAllIPSets {
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
			require.NoError(t, iMgr.AddReference(tt.metadata, testNetPolKey, NetPolType))
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
					{metadata: tt.metadata, members: members, netPolReferences: []string{testNetPolKey}},
				},
				toAddUpdateCache: dirtySets,
				toDeleteCache:    nil,
				setsForKernel:    dirtySets,
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

func TestAddReference(t *testing.T) {
	ref0 := "ref0" // for alreadyReferenced
	ref1 := "ref1"
	type args struct {
		cfg               *IPSetManagerCfg
		metadata          *IPSetMetadata
		refType           ReferenceType
		alreadyExisted    bool
		alreadyReferenced bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		*expectedInfo
	}{
		{
			name: "Apply Always: successfully add selector reference (set did not exist)",
			args: args{
				cfg:      applyAlwaysCfg,
				metadata: namespaceSet,
				refType:  SelectorType,
			},
			wantErr: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, selectorReferences: []string{ref1}},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet},
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
		},
		{
			name: "Apply Always: successfully add selector reference (set existed)",
			args: args{
				cfg:            applyAlwaysCfg,
				metadata:       namespaceSet,
				refType:        SelectorType,
				alreadyExisted: true,
			},
			wantErr: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, selectorReferences: []string{ref1}},
				},
			},
		},
		{
			name: "Apply Always: not a selector set (set did not exist)",
			args: args{
				cfg:      applyAlwaysCfg,
				metadata: list,
				refType:  SelectorType,
			},
			wantErr: true,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: list},
				},
				toAddUpdateCache: []*IPSetMetadata{list},
				setsForKernel:    []*IPSetMetadata{list},
			},
		},
		{
			name: "Apply Always: not a selector set (set existed)",
			args: args{
				cfg:            applyAlwaysCfg,
				metadata:       list,
				refType:        SelectorType,
				alreadyExisted: true,
			},
			wantErr: true,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: list},
				},
			},
		},
		{
			// already tested set (not) existing
			name: "Apply Always: successfully add netpol reference",
			args: args{
				cfg:            applyAlwaysCfg,
				metadata:       namespaceSet,
				refType:        NetPolType,
				alreadyExisted: true,
			},
			wantErr: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, netPolReferences: []string{ref1}},
				},
			},
		},
		{
			name: "Apply On Need: successfully add reference (set did not exist)",
			args: args{
				cfg:      applyOnNeedCfg,
				metadata: namespaceSet,
				refType:  SelectorType,
			},
			wantErr: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, selectorReferences: []string{ref1}},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet},
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
		},
		{
			name: "Apply On Need: successfully add reference (set existed but not already referenced)",
			args: args{
				cfg:            applyOnNeedCfg,
				metadata:       namespaceSet,
				refType:        SelectorType,
				alreadyExisted: true,
			},
			wantErr: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, selectorReferences: []string{ref1}},
				},
				toAddUpdateCache: []*IPSetMetadata{namespaceSet},
				setsForKernel:    []*IPSetMetadata{namespaceSet},
			},
		},
		{
			name: "Apply On Need: successfully add reference (set already referenced)",
			args: args{
				cfg:               applyOnNeedCfg,
				metadata:          namespaceSet,
				refType:           SelectorType,
				alreadyExisted:    true,
				alreadyReferenced: true,
			},
			wantErr: false,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: namespaceSet, selectorReferences: []string{ref0, ref1}},
				},
			},
		},
		{
			// best to test an unreferenced set to make sure it doesn't get added to the update cache
			name: "Apply On Need: not a selector set",
			args: args{
				cfg:      applyOnNeedCfg,
				metadata: list,
				refType:  SelectorType,
			},
			wantErr: true,
			expectedInfo: &expectedInfo{
				mainCache: []setMembers{
					{metadata: list},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()

			metadatas := []*IPSetMetadata{tt.args.metadata}

			var calls []testutils.TestCmd
			if tt.args.alreadyExisted && (tt.args.cfg.IPSetMode == ApplyAllIPSets || tt.args.alreadyReferenced) {
				calls = GetApplyIPSetsTestCalls(metadatas, nil)
			}
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(tt.args.cfg, ioShim)

			if tt.args.alreadyExisted {
				iMgr.CreateIPSets(metadatas)
				if tt.args.alreadyReferenced {
					require.NoError(t, iMgr.AddReference(tt.args.metadata, ref0, tt.args.refType), "alreadyReferenced and wantErr is not supported")
				}
				require.NoError(t, iMgr.ApplyIPSets())
			}

			err := iMgr.AddReference(tt.args.metadata, ref1, tt.args.refType)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assertExpectedInfo(t, iMgr, tt.expectedInfo)
		})
	}
}

func TestDeleteReferenceApplyAlways(t *testing.T) {
	metadata := namespaceSet
	type args struct {
		refType      ReferenceType
		setExists    bool
		hadReference bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "successfully delete selector reference",
			args: args{
				refType:      SelectorType,
				setExists:    true,
				hadReference: true,
			},
			wantErr: false,
		},
		{
			name: "successfully delete selector reference (wasn't referenced)",
			args: args{
				refType:      SelectorType,
				setExists:    true,
				hadReference: false,
			},
			wantErr: false,
		},
		{
			name: "successfully delete netpol reference",
			args: args{
				refType:      NetPolType,
				setExists:    true,
				hadReference: true,
			},
			wantErr: false,
		},
		{
			name: "successfully delete netpol reference (wasn't referenced)",
			args: args{
				refType:      NetPolType,
				setExists:    true,
				hadReference: false,
			},
			wantErr: false,
		},
		{
			name: "failure when set doesn't exist",
			args: args{
				refType:      SelectorType,
				hadReference: false,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// check semantics
			require.False(t, tt.args.setExists && tt.wantErr, "setExists and wantErr is not supported")

			ref := "ref"
			metrics.ReinitializeAll()

			var calls []testutils.TestCmd
			if tt.args.setExists {
				calls = GetApplyIPSetsTestCalls([]*IPSetMetadata{metadata}, nil)
			}
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)

			if tt.args.setExists {
				iMgr.CreateIPSets([]*IPSetMetadata{metadata})
				require.NoError(t, iMgr.AddReference(metadata, ref, tt.args.refType))
				require.NoError(t, iMgr.ApplyIPSets())
			}

			err := iMgr.DeleteReference(metadata.GetPrefixName(), ref, tt.args.refType)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			info := &expectedInfo{}
			if tt.args.setExists {
				setMember := setMembers{metadata: metadata}
				info.mainCache = []setMembers{setMember}
			}
			assertExpectedInfo(t, iMgr, info)
		})
	}
}

func TestDeleteReferenceApplyOnNeed(t *testing.T) {
	type referenceCount bool
	oneReference := referenceCount(false)
	twoReferences := referenceCount(true)
	tests := []struct {
		name            string
		numReferences   referenceCount
		shouldDeleteSet bool
	}{
		{
			name:            "Apply On Need: delete last reference",
			numReferences:   oneReference,
			shouldDeleteSet: true,
		},
		{
			name:            "Apply On Need: delete a reference (set still referenced after)",
			numReferences:   twoReferences,
			shouldDeleteSet: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metadata := namespaceSet
			ref0 := "ref0"
			ref1 := "ref1"

			metrics.ReinitializeAll()

			calls := GetApplyIPSetsTestCalls([]*IPSetMetadata{metadata}, nil)
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyOnNeedCfg, ioShim)

			iMgr.CreateIPSets([]*IPSetMetadata{metadata})
			if tt.numReferences == twoReferences {
				require.NoError(t, iMgr.AddReference(metadata, ref0, SelectorType))
			}
			require.NoError(t, iMgr.AddReference(metadata, ref1, SelectorType))
			require.NoError(t, iMgr.ApplyIPSets())
			require.NoError(t, iMgr.DeleteReference(metadata.GetPrefixName(), ref1, SelectorType))

			info := &expectedInfo{}
			s := setMembers{metadata: metadata}
			if tt.numReferences == twoReferences {
				s.selectorReferences = []string{ref0}
			}
			info.mainCache = []setMembers{s}
			if tt.shouldDeleteSet {
				info.toDeleteCache = []string{metadata.GetPrefixName()}
			}
			assertExpectedInfo(t, iMgr, info)
		})
	}
}

func TestMain(m *testing.M) {
	metrics.InitializeAll()

	exitCode := m.Run()

	os.Exit(exitCode)
}

func TestValidateIPSetMemberIP(t *testing.T) {
	tests := []struct {
		name    string
		ipblock string
		want    bool
	}{
		{
			name:    "cidr",
			ipblock: "172.17.0.0/16",
			want:    true,
		},
		{
			name:    "except ipblock",
			ipblock: "172.17.1.0/24 nomatch",
			want:    true,
		},
		{
			name:    "incorrect ip format",
			ipblock: "1234",
			want:    false,
		},
		{
			name:    "incorrect ip range",
			ipblock: "256.1.2.3",
			want:    false,
		},
		{
			name:    "empty cidr",
			ipblock: "",
			want:    false,
		},
		{
			name:    "ipv6",
			ipblock: "2345:0425:2CA1:0000:0000:0567:5673:23b5/24",
			want:    false,
		},
		{
			name:    "tcp",
			ipblock: "192.168.0.0/24,tcp:25227",
			want:    true,
		},
		{
			name:    "valid ip no cidr",
			ipblock: "10.0.0.0",
			want:    true,
		},
		{
			name:    "invalid cidr",
			ipblock: "10.0.0.1/33",
			want:    false,
		},
		{
			name:    "valid ip nomatch",
			ipblock: "192.168.0.1 nomatch",
			want:    true,
		},
		{
			name:    "valid ip tcp",
			ipblock: "192.168.0.1,tcp:25227",
			want:    true,
		},
		{
			name:    "ipv6 tcp",
			ipblock: "2345:0425:2CA1:0000:0000:0567:5673:23b5/24,tcp:25227",
			want:    false,
		},
		{
			name:    "ipv6 nomatch",
			ipblock: "2345:0425:2CA1:0000:0000:0567:5673:23b5 nomatch",
			want:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := validateIPSetMemberIP(tt.ipblock)
			require.Equal(t, tt.want, got)
		})
	}
}

func assertExpectedInfo(t *testing.T, iMgr *IPSetManager, info *expectedInfo) {
	// 1. assert cache contents
	// 1.1. make sure the main cache is equal, including members and references
	require.Equal(t, len(info.mainCache), len(iMgr.setMap), "main cache size mismatch")
	for _, setMembers := range info.mainCache {
		setName := setMembers.metadata.GetPrefixName()
		require.True(t, iMgr.exists(setName), "set %s not found in main cache", setName)
		set := iMgr.GetIPSet(setName)
		require.NotNil(t, set, "set %s should be non-nil", setName)
		require.Equal(t, util.GetHashedName(setName), set.HashedName, "HashedName mismatch")

		require.Equal(t, len(setMembers.members), len(set.IPPodKey)+len(set.MemberIPSets), "set %s member size mismatch", setName)
		for _, member := range setMembers.members {
			if member.kind == isHashMember {
				_, ok := set.IPPodKey[member.value]
				require.True(t, ok, "ip member %s not found in set %s", member.value, setName)
			} else {
				_, ok := set.MemberIPSets[member.value]
				require.True(t, ok, "set member %s not found in list %s", member.value, setName)
			}
		}

		require.Equal(t, len(setMembers.selectorReferences), len(set.SelectorReference), "set %s selector reference size mismatch", setName)
		for _, ref := range setMembers.selectorReferences {
			_, ok := set.SelectorReference[ref]
			require.True(t, ok, "selector reference %s not found in set %s", ref, setName)
		}

		require.Equal(t, len(setMembers.netPolReferences), len(set.NetPolReference), "set %s netpol reference size mismatch", setName)
		for _, ref := range setMembers.netPolReferences {
			_, ok := set.NetPolReference[ref]
			require.True(t, ok, "netpol reference %s not found in set %s", ref, setName)
		}
	}

	// 1.2. make sure the toAddOrUpdateCache is equal
	require.Equal(t, len(info.toAddUpdateCache), iMgr.dirtyCache.numSetsToAddOrUpdate(), "toAddUpdateCache size mismatch")
	for _, setMetadata := range info.toAddUpdateCache {
		setName := setMetadata.GetPrefixName()
		require.True(t, iMgr.dirtyCache.isSetToAddOrUpdate(setName), "set %s not in the toAddUpdateCache")
		require.True(t, iMgr.exists(setName), "set %s not in the main cache but is in the toAddUpdateCache", setName)
	}

	// 1.3. make sure the toDeleteCache is equal
	require.Equal(t, len(info.toDeleteCache), iMgr.dirtyCache.numSetsToDelete(), "toDeleteCache size mismatch")
	for _, setName := range info.toDeleteCache {
		require.True(t, iMgr.dirtyCache.isSetToDelete(setName), "set %s not found in toDeleteCache", setName)
	}

	// 1.4. assert kernel status of sets in the toAddOrUpdateCache
	for _, setMetadata := range info.setsForKernel {
		// check semantics
		require.True(t, iMgr.dirtyCache.isSetToAddOrUpdate(setMetadata.GetPrefixName()), "setsForKernel should be a subset of toAddUpdateCache")

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
	require.Equal(t, len(iMgr.setMap), numIPSetsInCache, "num ipsets mismatch")

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
	require.Equal(t, expectedNumEntriesInCache, numEntriesInCache, "incorrect num ipset entries")
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
