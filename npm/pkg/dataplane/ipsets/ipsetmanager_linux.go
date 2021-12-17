package ipsets

import (
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/parse"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
)

const (
	azureNPMPrefix = "azure-npm-"

	ipsetCommand        = "ipset"
	ipsetListFlag       = "list"
	ipsetNameFlag       = "--name"
	ipsetSaveFlag       = "save"
	ipsetRestoreFlag    = "restore"
	ipsetCreateFlag     = "-N"
	ipsetFlushFlag      = "-F"
	ipsetAddFlag        = "-A"
	ipsetDeleteFlag     = "-D"
	ipsetDestroyFlag    = "-X"
	ipsetExistFlag      = "--exist"
	ipsetNetHashFlag    = "nethash"
	ipsetSetListFlag    = "setlist"
	ipsetIPPortHashFlag = "hash:ip,port"
	ipsetMaxelemName    = "maxelem"
	ipsetMaxelemNum     = "4294967295"

	// constants for parsing ipset save
	createStringWithSpace = "create "
	space                 = " "
	addStringWithSpace    = "add "

	ipsetSetListString    = "list:set"
	ipsetNetHashString    = "hash:net"
	ipsetIPPortHashString = ipsetIPPortHashFlag

	// creator constants
	maxTryCount                    = 3
	destroySectionPrefix           = "delete"
	addOrUpdateSectionPrefix       = "add/update"
	ipsetRestoreLineFailurePattern = "Error in line (\\d+):"
)

var (
	// creator variables
	setDoesntExistDefinition       = ioutil.NewErrorDefinition("The set with the given name does not exist")
	setInUseByKernelDefinition     = ioutil.NewErrorDefinition("Set cannot be destroyed: it is in use by a kernel component")
	setAlreadyExistsDefinition     = ioutil.NewErrorDefinition("Set cannot be created: set with the same name already exists")
	memberSetDoesntExistDefinition = ioutil.NewErrorDefinition("Set to be added/deleted/tested as element does not exist")
)

/*
	based on ipset list output with azure-npm- prefix, create an ipset restore file where we flush all sets first, then destroy all sets

	overall error handling:
	- if flush fails because the set doesn't exist (should never happen because we're listing sets right before), then ignore it and the destroy
	- if flush fails otherwise, then add to destroyFailureCount and continue (aborting the destroy too)
	- if destroy fails because the set doesn't exist (should never happen since the flush operation would have worked), then ignore it
	- if destroy fails for another reason, then ignore it and add to destroyFailureCount and mark for reconcile (TODO)

	example:
		grep output:
			azure-npm-123456
			azure-npm-987654
			azure-npm-777777

		example restore file [flag meanings: -F (flush), -X (destroy)]:
			-F azure-npm-123456
			-F azure-npm-987654
			-F azure-npm-777777
			-X azure-npm-123456
			-X azure-npm-987654
			-X azure-npm-777777

	prometheus metrics:
		After this function, NumIPSets should be 0 or the number of NPM IPSets that existed and failed to be destroyed.
		When NPM restarts, Prometheus metrics will initialize at 0, but NPM IPSets may exist.
		We will reset ipset entry metrics if the restore succeeds whether or not some flushes/destroys failed (NOTE: this is different behavior than v1).
		If a flush fails, we could update the num entries for that set, but that would be a lot of overhead.
*/
func (iMgr *IPSetManager) resetIPSets() error {
	listCommand := iMgr.ioShim.Exec.Command(ipsetCommand, ipsetListFlag, ipsetNameFlag)
	grepCommand := iMgr.ioShim.Exec.Command(ioutil.Grep, azureNPMPrefix)
	azureIPSets, haveAzureIPSets, commandError := ioutil.PipeCommandToGrep(listCommand, grepCommand)
	if commandError != nil {
		return npmerrors.SimpleErrorWrapper("failed to run ipset list for resetting IPSets", commandError)
	}
	if !haveAzureIPSets {
		metrics.ResetNumIPSets()
		metrics.ResetIPSetEntries()
		return nil
	}
	creator, originalNumAzureSets, destroyFailureCount := iMgr.fileCreatorForReset(azureIPSets)
	restoreError := creator.RunCommandWithFile(ipsetCommand, ipsetRestoreFlag)
	if restoreError != nil {
		metrics.SetNumIPSets(originalNumAzureSets)
		// NOTE: the num entries for sets may be incorrect if the restore fails
		return npmerrors.SimpleErrorWrapper("failed to run ipset restore for resetting IPSets", restoreError)
	}
	if metrics.NumIPSetsIsPositive() {
		metrics.SetNumIPSets(*destroyFailureCount)
	} else {
		metrics.ResetNumIPSets()
	}
	metrics.ResetIPSetEntries() // NOTE: the num entries for sets that fail to flush may be incorrect after this
	return nil
}

// this needs to be a separate function because we need to check creator contents in UTs
func (iMgr *IPSetManager) fileCreatorForReset(ipsetListOutput []byte) (*ioutil.FileCreator, int, *int) {
	destroyFailureCount := 0
	creator := ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern)
	names := make([]string, 0)
	readIndex := 0
	var line []byte
	// flush all the sets and create a list of the sets so we can destroy them
	for readIndex < len(ipsetListOutput) {
		line, readIndex = parse.Line(readIndex, ipsetListOutput)
		hashedSetName := string(line)
		names = append(names, hashedSetName)
		// error handlers specific to resetting ipsets
		errorHandlers := []*ioutil.LineErrorHandler{
			{
				Definition: setDoesntExistDefinition,
				Method:     ioutil.ContinueAndAbortSection,
				Callback: func() {
					klog.Infof("[RESET-IPSETS] skipping flush and upcoming destroy for set %s since the set doesn't exist", hashedSetName)
				},
			},
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.ContinueAndAbortSection,
				Callback: func() {
					klog.Errorf("[RESET-IPSETS] marking flush and upcoming destroy for set %s as a failure due to unknown error", hashedSetName)
					destroyFailureCount++
					// TODO mark as a failure
				},
			},
		}
		sectionID := sectionID(destroySectionPrefix, hashedSetName)
		creator.AddLine(sectionID, errorHandlers, ipsetFlushFlag, hashedSetName) // flush set
	}

	// destroy all the sets
	for _, hashedSetName := range names {
		hashedSetName := hashedSetName // to appease go lint
		errorHandlers := []*ioutil.LineErrorHandler{
			// error handlers specific to resetting ipsets
			{
				Definition: setInUseByKernelDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Errorf("[RESET-IPSETS] marking destroy for set %s as a failure since the set is in use by a kernel component", hashedSetName)
					destroyFailureCount++
					// TODO mark the set as a failure and reconcile what iptables rule or ipset is referring to it
				},
			},
			{
				Definition: setDoesntExistDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Infof("[RESET-IPSETS] skipping destroy for set %s since the set does not exist", hashedSetName)
				},
			},
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Errorf("[RESET-IPSETS] marking destroy for set %s as a failure due to unknown error", hashedSetName)
					destroyFailureCount++
					// TODO mark the set as a failure and reconcile what iptables rule or ipset is referring to it
				},
			},
		}
		sectionID := sectionID(destroySectionPrefix, hashedSetName)
		creator.AddLine(sectionID, errorHandlers, ipsetDestroyFlag, hashedSetName) // destroy set
	}
	originalNumAzureSets := len(names)
	return creator, originalNumAzureSets, &destroyFailureCount
}

/*
overall error handling for ipset restore file.
ipset restore will apply all lines to the kernel before a failure, so when recovering from a line failure, we must skip the lines that were already applied.
below, "set" refers to either hashset or list, except in the sections for adding to (hash)set and adding to list

for flush/delete:
- abort the flush and delete calls if flush doesn't work
  - checks if set doesn't exist, but performs the same handling for any error
- skip the delete if it fails, and mark it as a failure (TODO)
  - checks if the set is in use by kernel component, but performs the same handling for any error

for create:
- abort create and add/delete calls if create doesn't work
  - checks if the set/list already exists, but performs the same handling for any error

for add to set:
- skip add if it fails

for add to list:
- skip the add if it fails, and mark it as a failure (TODO)
  - checks if the member set can't be added to a list because it doesn't exist, but performs the same handling for any error

for delete:
- skip the delete if it fails for any reason

overall format for ipset restore file:
	[creates]  (random order)
	[deletes and adds for sets already in the kernel] (in order of occurrence in save file, deletes first (in random order), then adds (in random order))
	[adds for new sets] (random order for sets and members)
	[flushes]  (random order)
	[destroys] (random order)

example where every set in add/update cache should have ip 1.2.3.4 and 2.3.4.5:
	save file showing current kernel state:
		create set-in-kernel-1 net:hash ...
		add set-in-kernel-1 1.2.3.4
		add set-in-kernel-1 8.8.8.8
		add set-in-kernel-1 9.9.9.9
		create set-in-kernel-2 net:hash ...
		add set-in-kernel-1 3.3.3.3

	restore file: [flag meanings: -F (flush), -X (destroy), -N (create), -D (delete), -A (add)]
		-N new-set-2
		-N set-in-kernel-2
		-N set-in-kernel-1
		-N new-set-1
		-N new-set-3
		-D set-in-kernel-1 8.8.8.8
		-D set-in-kernel-1 9.9.9.9
		-A set-in-kernel-1 2.3.4.5
		-D set-in-kernel-2 3.3.3.3
		-A set-in-kernel-2 2.3.4.5
		-A set-in-kernel-2 1.2.3.4
		-A new-set-2 1.2.3.4
		-A new-set-2 2.3.4.5
		-A new-set-1 2.3.4.5
		-A new-set-1 1.2.3.4
		-A new-set-3 1.2.3.4
		-A new-set-3 2.3.4.5
		-F set-to-delete2
		-F set-to-delete3
		-F set-to-delete1
		-X set-to-delete2
		-X set-to-delete3
		-X set-to-delete1

*/
func (iMgr *IPSetManager) applyIPSets() error {
	var saveFile []byte
	var saveError error
	if len(iMgr.toAddOrUpdateCache) > 0 {
		saveFile, saveError = iMgr.ipsetSave()
		if saveError != nil {
			return npmerrors.SimpleErrorWrapper("ipset save failed when applying ipsets", saveError)
		}
	}
	creator := iMgr.fileCreatorForApply(maxTryCount, saveFile)
	restoreError := creator.RunCommandWithFile(ipsetCommand, ipsetRestoreFlag)
	if restoreError != nil {
		return npmerrors.SimpleErrorWrapper("ipset restore failed when applying ipsets", restoreError)
	}
	return nil
}

func (iMgr *IPSetManager) ipsetSave() ([]byte, error) {
	command := iMgr.ioShim.Exec.Command(ipsetCommand, ipsetSaveFlag)
	grepCommand := iMgr.ioShim.Exec.Command(ioutil.Grep, azureNPMPrefix)
	saveFile, haveAzureSets, err := ioutil.PipeCommandToGrep(command, grepCommand)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to run ipset save", err)
	}
	if !haveAzureSets {
		return nil, nil
	}
	return saveFile, nil
}

func (iMgr *IPSetManager) fileCreatorForApply(maxTryCount int, saveFile []byte) *ioutil.FileCreator {
	creator := ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern) // TODO make the line failure pattern into a definition constant eventually

	// 1. create all sets first so we don't try to add a member set to a list if it hasn't been created yet
	for prefixedName := range iMgr.toAddOrUpdateCache {
		set := iMgr.setMap[prefixedName]
		iMgr.createSetForApply(creator, set)
		// NOTE: currently no logic to handle this scenario:
		// if a set in the toAddOrUpdateCache is in the kernel with the wrong type, then we'll try to create it, which will fail in the first restore call, but then be skipped in a retry
	}

	// 2. for dirty sets already in the kernel, update members (add members not in the kernel, and delete undesired members in the kernel)
	iMgr.updateDirtyKernelSets(saveFile, creator)

	// 3. for the remaining dirty sets, add their members to the kernel
	for prefixedName := range iMgr.toAddOrUpdateCache {
		set := iMgr.setMap[prefixedName]
		sectionID := sectionID(addOrUpdateSectionPrefix, prefixedName)
		if set.Kind == HashSet {
			for ip := range set.IPPodKey {
				iMgr.addMemberForApply(creator, set, sectionID, ip)
			}
		} else {
			for _, member := range set.MemberIPSets {
				iMgr.addMemberForApply(creator, set, sectionID, member.HashedName)
			}
		}
	}

	/*
		4. flush and destroy sets in the original delete cache

		We must perform this step after member deletions because of the following scenario:
		Suppose we want to destroy set A, which is referenced by list L. For set A to be in the toDeleteCache,
		we must have deleted the reference in list L, so list L is in the toAddOrUpdateCache. In step 2, we will delete this reference,
		but until then, set A is in use by a kernel component and can't be destroyed.
	*/
	// flush all sets first in case a set we're destroying is referenced by a list we're destroying
	for prefixedName := range iMgr.toDeleteCache {
		iMgr.flushSetForApply(creator, prefixedName)
	}
	for prefixedName := range iMgr.toDeleteCache {
		iMgr.destroySetForApply(creator, prefixedName)
	}
	return creator
}

// updates the creator (adds/deletes members) for dirty sets already in the kernel
// updates the toAddOrUpdateCache: after calling this function, the cache will only consist of sets to create
// error handling principal:
// - if contract with ipset save (or grep) is breaking, salvage what we can, take a snapshot (TODO), and log the failure
// - have a background process for sending/removing snapshots intermittently
func (iMgr *IPSetManager) updateDirtyKernelSets(saveFile []byte, creator *ioutil.FileCreator) {
	// map hashed names to prefixed names
	toAddOrUpdateHashedNames := make(map[string]string)
	for prefixedName := range iMgr.toAddOrUpdateCache {
		hashedName := iMgr.setMap[prefixedName].HashedName
		toAddOrUpdateHashedNames[hashedName] = prefixedName
	}

	klog.Infof("beginning to parse ipset save file:\nBEGIN-IPSET-SAVE-FILE-FOR-APPLY-IPSETS\n%s\nEND-IPSET-SAVE-FILE-FOR-APPLY-IPSETS", string(saveFile)) // TODO remove eventually

	// in each iteration, read a create line and any ensuing add lines
	readIndex := 0
	var line []byte
	if readIndex < len(saveFile) {
		line, readIndex = parse.Line(readIndex, saveFile)
		if !hasPrefix(line, createStringWithSpace) {
			klog.Errorf("expected a create line in ipset save file, but got the following line: %s", string(line))
			// TODO send error snapshot
			line, readIndex = nextCreateLine(readIndex, saveFile)
		}
	}
	for readIndex < len(saveFile) {
		// 1. get the hashed name
		lineAfterCreate := string(line[len(createStringWithSpace):])
		spaceSplitLineAfterCreate := strings.Split(lineAfterCreate, space)
		hashedName := spaceSplitLineAfterCreate[0]

		// 2. continue to the next create line if the set isn't in the toAddOrUpdateCache
		prefixedName, shouldModify := toAddOrUpdateHashedNames[hashedName]
		if !shouldModify {
			line, readIndex = nextCreateLine(readIndex, saveFile)
			continue
		}

		// 3. update the set from the kernel
		set := iMgr.setMap[prefixedName]
		// remove from the dirty cache so we don't add it later
		delete(iMgr.toAddOrUpdateCache, prefixedName)
		// mark the set as in the kernel
		delete(toAddOrUpdateHashedNames, hashedName)

		// 3.1 check for consistent type
		restOfLine := spaceSplitLineAfterCreate[1:]
		if haveTypeProblem(set, restOfLine) {
			// error logging happens in the helper function
			// TODO send error snapshot
			line, readIndex = nextCreateLine(readIndex, saveFile)
			continue
		}

		// 3.2 get desired members from cache
		var membersToAdd map[string]struct{}
		if set.Kind == HashSet {
			membersToAdd = make(map[string]struct{}, len(set.IPPodKey))
			for ip := range set.IPPodKey {
				membersToAdd[ip] = struct{}{}
			}
		} else {
			membersToAdd = make(map[string]struct{}, len(set.IPPodKey))
			for _, member := range set.MemberIPSets {
				membersToAdd[member.HashedName] = struct{}{}
			}
		}

		// 3.4 determine which members to add/delete
		membersToDelete := make(map[string]struct{})
		for readIndex < len(saveFile) {
			line, readIndex = parse.Line(readIndex, saveFile)
			if hasPrefix(line, createStringWithSpace) {
				break
			}
			if !hasPrefix(line, addStringWithSpace) {
				klog.Errorf("expected an add line, but got the following line: %s", string(line))
				// TODO send error snapshot
				line, readIndex = nextCreateLine(readIndex, saveFile)
				break
			}
			lineAfterAdd := string(line[len(addStringWithSpace):])
			spaceSplitLineAfterAdd := strings.Split(lineAfterAdd, space)
			parent := spaceSplitLineAfterAdd[0]
			if len(spaceSplitLineAfterAdd) != 2 || parent != hashedName {
				klog.Errorf("expected an add line for set %s in ipset save file, but got the following line: %s", hashedName, string(line))
				// TODO send error snapshot
				line, readIndex = nextCreateLine(readIndex, saveFile)
				break
			}
			member := spaceSplitLineAfterAdd[1]

			_, shouldKeep := membersToAdd[member]
			if shouldKeep {
				// member already in the kernel, so don't add it later
				delete(membersToAdd, member)
			} else {
				// member should be deleted from the kernel
				membersToDelete[member] = struct{}{}
			}
		}

		// 3.5 delete undesired members from restore file
		sectionID := sectionID(addOrUpdateSectionPrefix, prefixedName)
		for member := range membersToDelete {
			iMgr.deleteMemberForApply(creator, set, sectionID, member)
		}
		// 3.5 add new members to restore file
		for member := range membersToAdd {
			iMgr.addMemberForApply(creator, set, sectionID, member)
		}
	}
}

func nextCreateLine(originalReadIndex int, saveFile []byte) (createLine []byte, nextReadIndex int) {
	nextReadIndex = originalReadIndex
	for nextReadIndex < len(saveFile) {
		createLine, nextReadIndex = parse.Line(nextReadIndex, saveFile)
		if hasPrefix(createLine, createStringWithSpace) {
			return
		}
	}
	return
}

func haveTypeProblem(set *IPSet, restOfSpaceSplitCreateLine []string) bool {
	// TODO check type based on maxelem for hash sets? CIDR blocks have a different maxelem
	if len(restOfSpaceSplitCreateLine) == 0 {
		klog.Error("expected a type specification for the create line but received nothing after the set name")
		return true
	}
	typeString := restOfSpaceSplitCreateLine[0]
	switch typeString {
	case ipsetSetListString:
		if set.Kind != ListSet {
			lineString := fmt.Sprintf("create %s %s", set.HashedName, strings.Join(restOfSpaceSplitCreateLine, " "))
			klog.Errorf("expected to find a ListSet but have the line: %s", lineString)
			return true
		}
	case ipsetNetHashString:
		if set.Kind != HashSet || set.Type == NamedPorts {
			lineString := fmt.Sprintf("create %s %s", set.HashedName, strings.Join(restOfSpaceSplitCreateLine, " "))
			klog.Errorf("expected to find a non-NamedPorts HashSet but have the following line: %s", lineString)
			return true
		}
	case ipsetIPPortHashString:
		if set.Type != NamedPorts {
			lineString := fmt.Sprintf("create %s %s", set.HashedName, strings.Join(restOfSpaceSplitCreateLine, " "))
			klog.Errorf("expected to find a NamedPorts set but have the following line: %s", lineString)
			return true
		}
	default:
		klog.Errorf("unknown type string [%s] in line: %s", typeString, strings.Join(restOfSpaceSplitCreateLine, " "))
		return true
	}
	return false
}

func hasPrefix(line []byte, prefix string) bool {
	return len(line) >= len(prefix) && string(line[:len(prefix)]) == prefix
}

func (iMgr *IPSetManager) flushSetForApply(creator *ioutil.FileCreator, prefixedName string) {
	errorHandlers := []*ioutil.LineErrorHandler{
		{
			Definition: setDoesntExistDefinition,
			Method:     ioutil.ContinueAndAbortSection,
			Callback: func() {
				klog.Infof("skipping flush and upcoming destroy for set %s since the set doesn't exist", prefixedName)
			},
		},
		{
			Definition: ioutil.AlwaysMatchDefinition,
			Method:     ioutil.ContinueAndAbortSection,
			Callback: func() {
				klog.Errorf("skipping flush and upcoming destroy for set %s due to unknown error", prefixedName)
				// TODO mark as a failure
				// would this ever happen?
			},
		},
	}
	sectionID := sectionID(destroySectionPrefix, prefixedName)
	hashedName := util.GetHashedName(prefixedName)
	creator.AddLine(sectionID, errorHandlers, ipsetFlushFlag, hashedName) // flush set
}

func (iMgr *IPSetManager) destroySetForApply(creator *ioutil.FileCreator, prefixedName string) {
	errorHandlers := []*ioutil.LineErrorHandler{
		{
			Definition: setInUseByKernelDefinition,
			Method:     ioutil.Continue,
			Callback: func() {
				klog.Errorf("skipping destroy line for set %s since the set is in use by a kernel component", prefixedName)
				// TODO mark the set as a failure and reconcile what iptables rule or ipset is referring to it
			},
		},
		{
			Definition: ioutil.AlwaysMatchDefinition,
			Method:     ioutil.Continue,
			Callback: func() {
				klog.Errorf("skipping destroy line for set %s due to unknown error", prefixedName)
			},
		},
	}
	sectionID := sectionID(destroySectionPrefix, prefixedName)
	hashedName := util.GetHashedName(prefixedName)
	creator.AddLine(sectionID, errorHandlers, ipsetDestroyFlag, hashedName) // destroy set
}

func (iMgr *IPSetManager) createSetForApply(creator *ioutil.FileCreator, set *IPSet) {
	methodFlag := ipsetNetHashFlag
	if set.Kind == ListSet {
		methodFlag = ipsetSetListFlag
	} else if set.Type == NamedPorts {
		methodFlag = ipsetIPPortHashFlag
	}

	specs := []string{ipsetCreateFlag, set.HashedName, ipsetExistFlag, methodFlag}
	if set.Type == CIDRBlocks {
		specs = append(specs, ipsetMaxelemName, ipsetMaxelemNum)
	}

	prefixedName := set.Name // to appease golint complaints about function literal
	errorHandlers := []*ioutil.LineErrorHandler{
		{
			Definition: setAlreadyExistsDefinition,
			Method:     ioutil.ContinueAndAbortSection,
			Callback: func() {
				klog.Errorf("skipping create and any following adds/deletes for set %s since the set already exists with different specs", prefixedName)
				// TODO mark the set as a failure and handle this
			},
		},
		{
			Definition: ioutil.AlwaysMatchDefinition,
			Method:     ioutil.ContinueAndAbortSection,
			Callback: func() {
				klog.Errorf("skipping create and any following adds/deletes for set %s due to unknown error", prefixedName)
				// TODO same as above error handler
			},
		},
	}
	sectionID := sectionID(addOrUpdateSectionPrefix, prefixedName)
	creator.AddLine(sectionID, errorHandlers, specs...) // create set
}

func (iMgr *IPSetManager) deleteMemberForApply(creator *ioutil.FileCreator, set *IPSet, sectionID, member string) {
	errorHandlers := []*ioutil.LineErrorHandler{
		{
			Definition: ioutil.AlwaysMatchDefinition,
			Method:     ioutil.Continue,
			Callback: func() {
				klog.Errorf("skipping delete line for set %s due to unknown error", set.Name)
			},
		},
	}
	creator.AddLine(sectionID, errorHandlers, ipsetDeleteFlag, set.HashedName, member) // delete member
}

func (iMgr *IPSetManager) addMemberForApply(creator *ioutil.FileCreator, set *IPSet, sectionID, member string) {
	var errorHandlers []*ioutil.LineErrorHandler
	if set.Kind == ListSet {
		errorHandlers = []*ioutil.LineErrorHandler{
			{
				Definition: memberSetDoesntExistDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Errorf("skipping add of %s to list %s since the member doesn't exist", member, set.Name)
					// TODO reconcile
				},
			},
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Errorf("skipping add of %s to list %s due to unknown error", member, set.Name)
				},
			},
		}
	} else {
		errorHandlers = []*ioutil.LineErrorHandler{
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Errorf("skipping add line for hash set %s due to unknown error", set.Name)
				},
			},
		}
	}
	creator.AddLine(sectionID, errorHandlers, ipsetAddFlag, set.HashedName, member) // add member
}

func sectionID(prefix, prefixedName string) string {
	return fmt.Sprintf("%s-%s", prefix, prefixedName)
}
