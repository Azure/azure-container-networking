package ipsets

import (
	"errors"
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
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
	// ipsetReferCount keeps track of how many lists in the cache refer to this ipset
	ipsetReferCount int
	// kernelReferCount keeps track of how many lists in the kernel refer to this ipset
	kernelReferCount int
}

type IPSetMetadata struct {
	Name string
	Type SetType
}

// TranslatedIPSet is created by translation engine and provides IPSets used in
// network policy. Only 2 types of IPSets are generated with members:
// 1. CIDRBlocks IPSet
// 2. NestedLabelOfPod IPSet from multi value labels
// Members field holds member ipset names for NestedLabelOfPod and ip address ranges
// for CIDRBlocks IPSet
type TranslatedIPSet struct {
	Metadata *IPSetMetadata
	// Members holds member ipset names for NestedLabelOfPod and ip address ranges
	// for CIDRBlocks IPSet
	Members []string
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
	UnknownType SetType = 0
	// NameSpace IPSet is created to hold
	// ips of pods in a given NameSapce
	Namespace SetType = 1
	// KeyLabelOfNamespace IPSet is a list kind ipset
	// with members as ipsets of namespace with this Label Key
	KeyLabelOfNamespace SetType = 2
	// KeyValueLabelOfNamespace IPSet is a list kind ipset
	// with members as ipsets of namespace with this Label
	KeyValueLabelOfNamespace SetType = 3
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
	// Unknown const for unknown string
	Unknown string = "unknown"
)

var (
	setTypeName = map[SetType]string{
		UnknownType:              Unknown,
		Namespace:                "NameSpace",
		KeyLabelOfNamespace:      "KeyLabelOfNameSpace",
		KeyValueLabelOfNamespace: "KeyValueLabelOfNameSpace",
		KeyLabelOfPod:            "KeyLabelOfPod",
		KeyValueLabelOfPod:       "KeyValueLabelOfPod",
		NamedPorts:               "NamedPorts",
		NestedLabelOfPod:         "NestedLabelOfPod",
		CIDRBlocks:               "CIDRBlocks",
	}
	// ErrIPSetInvalidKind is returned when IPSet kind is invalid
	ErrIPSetInvalidKind = errors.New("invalid IPSet Kind")
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
	// UnknownKind is returned when kind is unknown
	UnknownKind SetKind = "unknown"
)

// ReferenceType specifies the kind of reference for an IPSet
type ReferenceType string

// Possible ReferenceTypes
const (
	SelectorType ReferenceType = "Selector"
	NetPolType   ReferenceType = "NetPol"
)

func NewIPSet(setMetadata *IPSetMetadata) *IPSet {
	prefixedName := setMetadata.GetPrefixName()
	set := &IPSet{
		Name:       prefixedName,
		HashedName: util.GetHashedName(prefixedName),
		SetProperties: SetProperties{
			Type: setMetadata.Type,
			Kind: GetSetKind(setMetadata.Type),
		},
		// Map with Key as Network Policy name to to emulate set
		// and value as struct{} for minimal memory consumption
		SelectorReference: make(map[string]struct{}),
		// Map with Key as Network Policy name to to emulate set
		// and value as struct{} for minimal memory consumption
		NetPolReference:  make(map[string]struct{}),
		ipsetReferCount:  0,
		kernelReferCount: 0,
	}
	if set.Kind == HashSet {
		set.IPPodKey = make(map[string]string)
	} else {
		set.MemberIPSets = make(map[string]*IPSet)
	}
	return set
}

// NewIPSetMetadata is used for controllers to send in skeleton ipsets to DP
func NewIPSetMetadata(name string, setType SetType) *IPSetMetadata {
	set := &IPSetMetadata{
		Name: name,
		Type: setType,
	}
	return set
}

func (setMetadata *IPSetMetadata) GetPrefixName() string {
	switch setMetadata.Type {
	case CIDRBlocks:
		return fmt.Sprintf("%s%s", util.CIDRPrefix, setMetadata.Name)
	case Namespace:
		return fmt.Sprintf("%s%s", util.NamespacePrefix, setMetadata.Name)
	case NamedPorts:
		return fmt.Sprintf("%s%s", util.NamedPortIPSetPrefix, setMetadata.Name)
	case KeyLabelOfPod:
		return fmt.Sprintf("%s%s", util.PodLabelPrefix, setMetadata.Name)
	case KeyValueLabelOfPod:
		return fmt.Sprintf("%s%s", util.PodLabelPrefix, setMetadata.Name)
	case KeyLabelOfNamespace:
		return fmt.Sprintf("%s%s", util.NamespaceLabelPrefix, setMetadata.Name)
	case KeyValueLabelOfNamespace:
		return fmt.Sprintf("%s%s", util.NamespaceLabelPrefix, setMetadata.Name)
	case NestedLabelOfPod:
		return fmt.Sprintf("%s%s", util.NestedLabelPrefix, setMetadata.Name)
	case UnknownType: // adding this to appease golint
		return Unknown
	default:
		return Unknown
	}
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

// ShallowCompare check if the properties of IPSets are same
func (set *IPSet) ShallowCompare(newSet *IPSet) bool {
	if set.Name != newSet.Name {
		return false
	}
	if set.Kind != newSet.Kind {
		return false
	}
	if set.Type != newSet.Type {
		return false
	}
	return true
}

// Compare checks if two ipsets are same
func (set *IPSet) Compare(newSet *IPSet) bool {
	if set.Name != newSet.Name {
		return false
	}
	if set.Kind != newSet.Kind {
		return false
	}
	if set.Type != newSet.Type {
		return false
	}
	if set.Kind == HashSet {
		if len(set.IPPodKey) != len(newSet.IPPodKey) {
			return false
		}
		for podIP := range set.IPPodKey {
			if _, ok := newSet.IPPodKey[podIP]; !ok {
				return false
			}
		}
	} else {
		if len(set.MemberIPSets) != len(newSet.MemberIPSets) {
			return false
		}
		for _, memberSet := range set.MemberIPSets {
			if _, ok := newSet.MemberIPSets[memberSet.HashedName]; !ok {
				return false
			}
		}
	}
	return true
}

func GetSetKind(setType SetType) SetKind {
	switch setType {
	case CIDRBlocks:
		return HashSet
	case Namespace:
		return HashSet
	case NamedPorts:
		return HashSet
	case KeyLabelOfPod:
		return HashSet
	case KeyValueLabelOfPod:
		return HashSet
	case KeyLabelOfNamespace:
		return ListSet
	case KeyValueLabelOfNamespace:
		return ListSet
	case NestedLabelOfPod:
		return ListSet
	case UnknownType: // adding this to appease golint
		return UnknownKind
	default:
		return UnknownKind
	}
}

func (set *IPSet) incIPSetReferCount() {
	set.ipsetReferCount++
}

func (set *IPSet) decIPSetReferCount() {
	if set.ipsetReferCount == 0 {
		return
	}
	set.ipsetReferCount--
}

func (set *IPSet) incKernelReferCount() {
	set.kernelReferCount++
}

func (set *IPSet) decKernelReferCount() {
	if set.kernelReferCount == 0 {
		return
	}
	set.kernelReferCount--
}

func (set *IPSet) addReference(referenceName string, referenceType ReferenceType) {
	switch referenceType {
	case SelectorType:
		set.SelectorReference[referenceName] = struct{}{}
	case NetPolType:
		set.NetPolReference[referenceName] = struct{}{}
	default:
		log.Logf("IPSet_addReference: encountered unknown ReferenceType")
	}
}

func (set *IPSet) deleteReference(referenceName string, referenceType ReferenceType) {
	switch referenceType {
	case SelectorType:
		delete(set.SelectorReference, referenceName)
	case NetPolType:
		delete(set.NetPolReference, referenceName)
	default:
		log.Logf("IPSet_deleteReference: encountered unknown ReferenceType")
	}
}

func (set *IPSet) shouldBeInKernel() bool {
	return set.usedByNetPol() || set.referencedInKernel()
}

func (set *IPSet) canBeDeleted() bool {
	return !set.usedByNetPol() &&
		!set.referencedInList() &&
		len(set.MemberIPSets) == 0 &&
		len(set.IPPodKey) == 0
}

// usedByNetPol check if an IPSet is referred in network policies.
func (set *IPSet) usedByNetPol() bool {
	return len(set.SelectorReference) > 0 ||
		len(set.NetPolReference) > 0
}

func (set *IPSet) referencedInList() bool {
	return set.ipsetReferCount > 0
}

func (set *IPSet) referencedInKernel() bool {
	return set.kernelReferCount > 0
}

// panics if set is not a list set
func (set *IPSet) hasMember(memberName string) bool {
	_, isMember := set.MemberIPSets[memberName]
	return isMember
}

func (set *IPSet) getSetIntersection(existingIntersection map[string]struct{}) (map[string]struct{}, error) {
	if !set.canSetBeSelectorIPSet() {
		return nil, npmerrors.Errorf(
			npmerrors.IPSetIntersection,
			false,
			fmt.Sprintf("[IPSet] Selector IPSet cannot be of type %s", set.Type.String()))
	}
	newIntersectionMap := make(map[string]struct{})
	for ip := range set.IPPodKey {
		if _, ok := existingIntersection[ip]; ok {
			newIntersectionMap[ip] = struct{}{}
		}
	}

	return newIntersectionMap, nil
}

func (set *IPSet) canSetBeSelectorIPSet() bool {
	return (set.Type == KeyLabelOfPod ||
		set.Type == KeyValueLabelOfPod ||
		set.Type == Namespace ||
		set.Type == NestedLabelOfPod)
}
