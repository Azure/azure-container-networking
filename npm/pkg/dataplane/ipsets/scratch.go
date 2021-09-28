package ipsets

// PROBLEM: suppose a set is referenced by 2 lists.
// when removing the set from a single list, don't know if we should remove it from the kernel because we don't konw if the 2nd list is in use by the kernel
// FIX: make the refer count an in-kernel count

/*
references stuff

func (iMgr *IPSetManager) AddSelectorReference(setName string, selectorName string) error {
	if err := iMgr.checkIfExists(setName, "FIXME"); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	iMgr.addReference(set, selectorName, set.addSelectorReference)
	return nil
}

func (iMgr *IPSetManager) DeleteSelectorReference(setName string, selectorName string) error {
	if err := iMgr.checkIfExists(setName, "FIXME"); err != nil {
		return err
	}
	set := iMgr.setMap[setName]

	return nil
}

func (iMgr *IPSetManager) AddNetPolReference(setName string, netPolName string) error {
	if err := iMgr.checkIfExists(setName, "FIXME"); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	iMgr.addReference(set, netPolName, set.addNetPolReference)
	return nil
}

func (iMgr *IPSetManager) DeleteNetPolReference(setName string, netPolName string) error {
	if err := iMgr.checkIfExists(setName, "FIXME"); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	set.deleteNetPolReference(netPolName)
	return nil
}

// panics if set doesn't exist
func (iMgr *IPSetManager) getSelectorReferences(setName string) map[string]struct{} {
	return iMgr.setMap[setName].SelectorReference
}

// panics if set doesn't exist
func (iMgr *IPSetManager) getNetPolReferences(setName string) map[string]struct{} {
	return iMgr.setMap[setName].NetPolReference
}

func (iMgr *IPSetManager) addReference(setName, referenceName string, getReferences func() map[string]struct{}) error {
	if err := iMgr.checkIfExists(setName, "FIXME"); err != nil {
		return err
	}
	set := iMgr.setMap[setName]
	wasInKernel := set.shouldBeInKernel()
	references := getReferences()
	references[referenceName] = struct{}{}
	if !wasInKernel {
		iMgr.addSetAndMembersToKernel(set)
	}
	return nil
}

func (iMgr *IPSetManager) deleteReference(set *IPSet, referenceName string, deleteFunction func(string)) {
	wasInKernel := set.shouldBeInKernel()
	deleteFunction(referenceName)
	if wasInKernel {
		iMgr.deleteSetAndMembersFromKernel(set)
	}
}


*/

/*


// func (iMgr *IPSetManager) updateIPSet(set *IPSet) error {
// 	oldSet := iMgr.setMap[set.Name]
// 	if !haveSameSetProperties(set, oldSet) {
// 		return fmt.Errorf("ipset already exists and has different set properties")
// 	}

// 	if haveSameSetContents(set, oldSet) {
// 		return nil
// 	}

// 	iMgr.setMap[set.Name] = set
// 	// TODO handle list members here

// 	if !set.shouldBeInKernel() && oldSet.shouldBeInKernel() {
// 		iMgr.deleteFromKernel(set.Name)
// 		return nil
// 	}
// 	if set.shouldBeInKernel() && !oldSet.shouldBeInKernel() {
// 		iMgr.addToKernel(set.Name)
// 		return nil
// 	}

// 	iMgr.modifyCacheIfInKernel(set.Name)
// 	return nil
// }


func haveSameSetProperties(set1 *IPSet, set2 *IPSet) bool {
	return set1.SetProperties.Type == set2.SetProperties.Type &&
		set1.SetProperties.Kind == set2.SetProperties.Kind
}

func haveSameSetContents(set1 *IPSet, set2 *IPSet) bool {
	return haveSameCounts(set1, set2) ||
		haveSameIPPodKeys(set1, set2) ||
		haveSameMemberIPsets(set1, set2)
}

func haveSameCounts(set1 *IPSet, set2 *IPSet) bool {
	return set1.ipsetReferCount == set2.ipsetReferCount &&
		len(set1.NetPolReference) == len(set2.NetPolReference) &&
		len(set1.SelectorReference) == len(set2.SelectorReference) &&
		len(set1.IPPodKey) == len(set2.IPPodKey) &&
		len(set1.MemberIPSets) == len(set2.MemberIPSets)
}

// assumes sets have same number of IPs
func haveSameIPPodKeys(set1 *IPSet, set2 *IPSet) bool {
	for ip, podKey1 := range set1.IPPodKey {
		podKey2, exists := set2.IPPodKey[ip]
		if !exists || podKey1 != podKey2 {
			return false
		}
	}
	return true
}

// assumes sets have same number of members
func haveSameMemberIPsets(set1 *IPSet, set2 *IPSet) bool {
	for memberName := range set1.MemberIPSets {
		_, exists := set2.MemberIPSets[memberName]
		if !exists {
			return false
		}
	}
	return true
}

*/

// calls := []testutils.TestCmd{
// 	{Cmd: []string{util.Ipset, util.IpsetCreationFlag, util.GetHashedName(testListName), util.IpsetSetListFlag}},
// 	{Cmd: []string{util.Ipset, util.IpsetCreationFlag, util.GetHashedName(testListName), util.IpsetSetListFlag}},
// }

// fexec := testutils.GetFakeExecWithScripts(calls)
// linuxExec = fexec

/*
lockFile, err := getIPSetLockFile()
if err != nil {
	return fmt.Errorf("failed to get IPSet lock file with error: %w", err)
}
defer closeLockFile(lockFile)


func getIPSetLockFile() (*os.File, error) {
return nil, nil // no lock file for ipset (will need for iptables equivalent)
}

func closeLockFile(fileCreator *os.File) {
// do nothing for ipset (will need for iptables equivalent)
}
*/

// OLD STUFF
/*
	echoFile := linuxExec.Command("printf", file.toString()) // TODO util constant for echo
	restoreCommand := linuxExec.Command(util.Ipset, util.IpsetRestoreFlag)
	pipe, err := echoFile.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error encountered while piping results of echoing restore file: %w", err)
	}
	defer pipe.Close()
	restoreCommand.SetStdin(pipe)
	// restoreCommand.Stdin = pipe

	if err = echoFile.Start(); err != nil {
		return fmt.Errorf("error encountered while echoing restore file: %w", err)
	}

	defer echoFile.Wait() // TODO is this necessary? "Without this wait, defunct iptable child process are created"

	if output, err := restoreCommand.CombinedOutput(); err != nil { // does output specify errors?
		fmt.Println("OUTPUT")
		fmt.Println(output) // FIXME
		return fmt.Errorf("error encountered while restoring ipsets: %w", err)
	}

	return nil
*/
