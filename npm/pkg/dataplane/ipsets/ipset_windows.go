package ipsets

// isIPAffiliated determines whether an PodIP belongs to the set or its member sets in the case of a list set.
// This method and GetSetContents are good examples of how the ipset struct may have been better designed
// as an interface with hash and list implementations. Not worth it to redesign though.
func (set *IPSet) isIPAffiliated(ip, podKey string) bool {
	if set.Kind == HashSet {
		if key, ok := set.IPPodKey[ip]; ok && key == podKey {
			return true
		}
	}
	for _, memberSet := range set.MemberIPSets {
		if key, ok := memberSet.IPPodKey[ip]; ok && key == podKey {
			return true
		}
	}
	return false
}
