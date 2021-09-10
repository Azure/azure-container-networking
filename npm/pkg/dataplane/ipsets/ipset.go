package ipsets

import (
	"errors"

	"github.com/Azure/azure-container-networking/npm/util"
)

type IPSet struct {
	Name       string
	HashedName string
	// SetProperties embedding set properties
	SetProperties
	// IpPodKey is used for setMaps to store Ips and ports as keys
	// and podKey as value
	IPPodKey map[string]string
	// This is used for listMaps to store child IP Sets
	MemberIPSets map[string]*IPSet
	// Using a map to emulate set and value as struct{} for
	// minimal memory consumption
	// SelectorReference holds networkpolicy names where this IPSet
	// is being used in PodSelector and NameSpace
	SelectorReference map[string]struct{}
	// NetPolReference holds networkpolicy names where this IPSet
	// is being referred as part of rules
	NetPolReference map[string]struct{}
	// IpsetReferCount keeps count of 2nd level Nested IPSets
	// with member as this IPSet
	IpsetReferCount int
}

type SetProperties struct {
	// Stores type of ip grouping
	Type SetType
	// Stores kind of ipset in dataplane
	Kind SetKind
}

type SetType int8

const (
	// Unknown SetType
	Unknown SetType = 0
	// NameSpace IPSet is created to hold
	// ips of pods in a given NameSapce
	NameSpace SetType = 1
	// KeyLabelOfNameSpace IPSet is a list kind ipset
	// with members as ipsets of namespace with this Label Key
	KeyLabelOfNameSpace SetType = 2
	// KeyValueLabelOfNameSpace IPSet is a list kind ipset
	// with members as ipsets of namespace with this Label
	KeyValueLabelOfNameSpace SetType = 3
	// KeyLabelOfPod IPSet contains IPs of Pods with this Label Key
	KeyLabelOfPod SetType = 4
	// KeyValueLabelOfPod IPSet contains IPs of Pods with this Label
	KeyValueLabelOfPod SetType = 5
	// NamedPorts IPSets contains a given namedport
	NamedPorts SetType = 6
	// NestedLabelOfPod is derived for multivalue matchexpressions
	NestedLabelOfPod SetType = 7
	// CIDRBlocks holds CIDR blocks
	CIDRBlocks SetType = 8
)

var (
	setTypeName = map[SetType]string{
		Unknown:                  "Unknown",
		NameSpace:                "NameSpace",
		KeyLabelOfNameSpace:      "KeyLabelOfNameSpace",
		KeyValueLabelOfNameSpace: "KeyValueLabelOfNameSpace",
		KeyLabelOfPod:            "KeyLabelOfPod",
		KeyValueLabelOfPod:       "KeyValueLabelOfPod",
		NamedPorts:               "NamedPorts",
		NestedLabelOfPod:         "NestedLabelOfPod",
		CIDRBlocks:               "CIDRBlocks",
	}
	// ErrIPSetInvalidKind is returned when IPSet kind is invalid
	ErrIPSetInvalidKind = errors.New("Invalid IPSet Kind")
)

func (x SetType) String() string {
	return setTypeName[x]
}

type SetKind string

const (
	// ListSet is of kind list with members as other IPSets
	ListSet SetKind = "list"
	// HashSet is of kind hashset with members as IPs and/or port
	HashSet SetKind = "set"
)

func NewIPSet(name string, setType SetType) *IPSet {
	set := &IPSet{
		Name:       name,
		HashedName: util.GetHashedName(name),
		SetProperties: SetProperties{
			Type: setType,
			Kind: getSetKind(setType),
		},
		// Map with Key as Network Policy name to to emulate set
		// and value as struct{} for minimal memory consumption
		SelectorReference: make(map[string]struct{}),
		// Map with Key as Network Policy name to to emulate set
		// and value as struct{} for minimal memory consumption
		NetPolReference: make(map[string]struct{}),
		IpsetReferCount: 0,
	}
	if set.Kind == HashSet {
		set.IPPodKey = make(map[string]string)
	} else {
		set.MemberIPSets = make(map[string]*IPSet)
	}
	return set
}

func (set *IPSet) GetSetContents() ([]string, error) {
	switch set.Kind {
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
		return []string{}, ErrIPSetInvalidKind
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
	return len(set.SelectorReference) > 0 &&
		len(set.NetPolReference) > 0
}
