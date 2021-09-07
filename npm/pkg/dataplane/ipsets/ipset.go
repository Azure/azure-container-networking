package ipsets

import (
	"fmt"

	"github.com/Azure/azure-container-networking/npm/util"
)

type IPSet struct {
	Name       string
	HashedName string
	Properties SetProperties
	// IpPodKey is used for setMaps to store Ips and ports as keys
	// and podKey as value
	IPPodKey map[string]string
	// This is used for listMaps to store child IP Sets
	MemberIPSets map[string]*IPSet
	// Using a map to emulate set and value as struct{} for
	// minimal memory consumption
	SelectorReference map[string]struct{}
	NetPolReference   map[string]struct{}
	IpsetReferCount   int
}

type SetProperties struct {
	// Stores type of ip grouping
	Type SetType
	// Stores kind of ipset in dataplane
	Kind SetKind
}

type SetType int32

const (
	Unknown                  SetType = 0
	NameSpace                SetType = 1
	KeyLabelOfNameSpace      SetType = 2
	KeyValueLabelOfNameSpace SetType = 3
	KeyLabelOfPod            SetType = 4
	KeyValueLabelOfPod       SetType = 5
	NamedPorts               SetType = 6
	NestedLabelOfPod         SetType = 7
	CIDRBlocks               SetType = 8
)

var SetTypeName = map[int32]string{
	0: "Unknown",
	1: "NameSpace",
	2: "KeyLabelOfNameSpace",
	3: "KeyValueLabelOfNameSpace",
	4: "KeyLabelOfPod",
	5: "KeyValueLabelOfPod",
	6: "NamedPorts",
	7: "NestedLabelOfPod",
	8: "CIDRBlocks",
}

var SetTypeValue = map[string]int32{
	"Unknown":                  0,
	"NameSpace":                1,
	"KeyLabelOfNameSpace":      2,
	"KeyValueLabelOfNameSpace": 3,
	"KeyLabelOfPod":            4,
	"KeyValueLabelOfPod":       5,
	"NamedPorts":               6,
	"NestedLabelOfPod":         7,
	"CIDRBlocks":               8,
}

func (x SetType) String() string {
	return SetTypeName[int32(x)]
}

func GetSetType(x string) SetType {
	return SetType(SetTypeValue[x])
}

type SetKind string

const (
	ListSet SetKind = "list"
	HashSet SetKind = "set"
)

func NewIPSet(name string, setType SetType) *IPSet {
	set := &IPSet{
		Name:       name,
		HashedName: util.GetHashedName(name),
		Properties: SetProperties{
			Type: setType,
			Kind: getSetKind(setType),
		},
		// Using a map to emulate set and value as struct{} for
		// minimal memory consumption
		SelectorReference: make(map[string]struct{}),
		NetPolReference:   make(map[string]struct{}),
		IpsetReferCount:   0,
	}
	if set.Properties.Kind == HashSet {
		set.IPPodKey = make(map[string]string)
	} else {
		set.MemberIPSets = make(map[string]*IPSet)
	}
	return set
}

func (set *IPSet) GetSetContents() ([]string, error) {
	switch set.Properties.Kind {
	case HashSet:
		i := 0
		contents := make([]string, len(set.IPPodKey))
		for podIP := range set.IPPodKey {
			contents[i] = podIP
			i++
		}
		return contents, nil
	case ListSet:
		i := 0
		contents := make([]string, len(set.MemberIPSets))
		for _, memberSet := range set.MemberIPSets {
			contents[i] = memberSet.HashedName
			i++
		}
		return contents, nil
	default:
		return []string{}, fmt.Errorf("Unknown set type %s", set.Properties.Type.String())
	}
}

func getSetKind(setType SetType) SetKind {
	switch setType {
	case CIDRBlocks:
		return HashSet
	case NameSpace:
		return HashSet
	case NamedPorts:
		return HashSet
	case KeyLabelOfPod:
		return HashSet
	case KeyValueLabelOfPod:
		return HashSet
	case KeyLabelOfNameSpace:
		return ListSet
	case KeyValueLabelOfNameSpace:
		return ListSet
	case NestedLabelOfPod:
		return ListSet
	case Unknown: // adding this to appease golint
		return "unknown"
	default:
		return "unknown"
	}
}

func (set *IPSet) AddMemberIPSet(memberIPSet *IPSet) {
	set.MemberIPSets[memberIPSet.Name] = memberIPSet
}

func (set *IPSet) IncIpsetReferCount() {
	set.IpsetReferCount++
}

func (set *IPSet) DecIpsetReferCount() {
	if set.IpsetReferCount == 0 {
		return
	}
	set.IpsetReferCount--
}

func (set *IPSet) AddSelectorReference(netPolName string) {
	set.SelectorReference[netPolName] = struct{}{}
}

func (set *IPSet) DeleteSelectorReference(netPolName string) {
	delete(set.SelectorReference, netPolName)
}

func (set *IPSet) AddNetPolReference(netPolName string) {
	set.NetPolReference[netPolName] = struct{}{}
}

func (set *IPSet) DeleteNetPolReference(netPolName string) {
	delete(set.NetPolReference, netPolName)
}

func (set *IPSet) CanBeDeleted() bool {
	return len(set.SelectorReference) == 0 &&
		len(set.NetPolReference) == 0 &&
		set.IpsetReferCount == 0 &&
		len(set.MemberIPSets) == 0 &&
		len(set.IPPodKey) == 0
}

// UsedByNetPol check if an IPSet is referred in network policies.
func (set *IPSet) UsedByNetPol() bool {
	return len(set.SelectorReference) >= 0 &&
		len(set.NetPolReference) >= 0
}
