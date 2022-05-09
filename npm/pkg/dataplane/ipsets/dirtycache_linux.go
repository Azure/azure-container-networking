package ipsets

type memberDiff struct {
	membersToAdd    map[string]struct{}
	membersToDelete map[string]struct{}
}

func newMemberDiff() *memberDiff {
	return &memberDiff{
		membersToAdd:    make(map[string]struct{}),
		membersToDelete: make(map[string]struct{}),
	}
}

func diffOnCreate(set *IPSet) *memberDiff {
	// mark all current members as membersToAdd
	var members map[string]struct{}
	if set.Kind == HashSet {
		members = make(map[string]struct{}, len(set.IPPodKey))
		for ip := range set.IPPodKey {
			members[ip] = struct{}{}
		}
	} else {
		members = make(map[string]struct{}, len(set.MemberIPSets))
		for _, memberSet := range set.MemberIPSets {
			members[memberSet.HashedName] = struct{}{}
		}
	}
	return &memberDiff{
		membersToAdd:    members,
		membersToDelete: make(map[string]struct{}),
	}
}

func (diff *memberDiff) addMember(member string) {
	_, ok := diff.membersToDelete[member]
	if ok {
		delete(diff.membersToDelete, member)
	} else {
		diff.membersToAdd[member] = struct{}{}
	}
}

func (diff *memberDiff) deleteMember(member string) {
	_, ok := diff.membersToAdd[member]
	if ok {
		delete(diff.membersToAdd, member)
	} else {
		diff.membersToDelete[member] = struct{}{}
	}
}

func (diff *memberDiff) removeMemberFromDiffToAdd(member string) {
	delete(diff.membersToAdd, member)
}

func (diff *memberDiff) resetMembersToAdd() {
	diff.membersToAdd = make(map[string]struct{})
}
