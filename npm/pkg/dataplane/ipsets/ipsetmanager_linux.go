package ipsets

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/parse"
	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"github.com/Azure/azure-container-networking/npm/util/ioutil"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

const (
	ipsetFlushAndDestroyString = "ipset flush && ipset destroy"

	azureNPMPrefix        = "azure-npm-"
	azureNPMRegex         = "azure-npm-\\d+"
	positiveRefsRegex     = "References: [1-9]"
	referenceGrepLookBack = "5"
	maxLinesToPrint       = 10

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
	maxTryCount                    = 5
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

	NOTE: the behavior has changed to run two separate restore files. The first to flush all, the second to destroy all. In between restores,
	we determine if there are any sets with leaked ipset reference counts. We ignore destroys for those sets in-line with v1.

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
	if success := iMgr.resetWithoutRestore(); success {
		return nil
	}

	// get current NPM ipsets
	listNamesCommand := iMgr.ioShim.Exec.Command(ipsetCommand, ipsetListFlag, ipsetNameFlag)
	grepCommand := iMgr.ioShim.Exec.Command(ioutil.Grep, azureNPMPrefix)
	klog.Infof("running this command while resetting ipsets: [%s %s %s | %s %s]", ipsetCommand, ipsetListFlag, ipsetNameFlag, ioutil.Grep, azureNPMRegex)
	azureIPSets, haveAzureNPMIPSets, commandError := ioutil.PipeCommandToGrep(listNamesCommand, grepCommand)
	if commandError != nil {
		return npmerrors.SimpleErrorWrapper("failed to run ipset list for resetting IPSets (prometheus metrics may be off now)", commandError)
	}
	if !haveAzureNPMIPSets {
		return nil
	}

	// flush all NPM sets
	creator, names, failedNames := iMgr.fileCreatorForFlushAll(azureIPSets)
	restoreError := creator.RunCommandWithFile(ipsetCommand, ipsetRestoreFlag)
	if restoreError != nil {
		klog.Errorf(
			"failed to flush all ipsets (prometheus metrics may be off now). originalNumAzureSets: %d. failed flushes: %+v. err: %v",
			len(names), failedNames, restoreError,
		)
		return npmerrors.SimpleErrorWrapper("failed to run ipset restore while flushing all for resetting IPSets", restoreError)
	}

	// destroy all NPM sets
	creator, destroyFailureCount := iMgr.fileCreatorForDestroyAll(names, failedNames, iMgr.setsWithReferences())
	destroyError := creator.RunCommandWithFile(ipsetCommand, ipsetRestoreFlag)
	if destroyError != nil {
		klog.Errorf(
			"failed to destroy all ipsets (prometheus metrics may be off now). destroyFailureCount %d. err: %v",
			destroyFailureCount, destroyError,
		)
		return npmerrors.SimpleErrorWrapper("failed to run ipset restore while destroying all for resetting IPSets", destroyError)
	}
	return nil
}

// resetWithoutRestore will return true (success) if able to reset without restore
func (iMgr *IPSetManager) resetWithoutRestore() bool {
	listNamesCommand := iMgr.ioShim.Exec.Command(ipsetCommand, ipsetListFlag, ipsetNameFlag)
	grepCommand := iMgr.ioShim.Exec.Command(ioutil.Grep, ioutil.GrepQuietFlag, ioutil.GrepAntiMatchFlag, azureNPMPrefix)
	commandString := fmt.Sprintf(" [%s %s %s | %s %s %s %s]", ipsetCommand, ipsetListFlag, ipsetNameFlag, ioutil.Grep, ioutil.GrepQuietFlag, ioutil.GrepAntiMatchFlag, azureNPMPrefix)
	klog.Infof("running this command while resetting ipsets: [%s]", commandString)
	_, haveNonAzureNPMIPSets, commandError := ioutil.PipeCommandToGrep(listNamesCommand, grepCommand)
	if commandError != nil {
		metrics.SendErrorLogAndMetric(util.IpsmID, "failed to determine if there were non-azure sets while resetting. err: %v", commandError)
		return false
	}
	if haveNonAzureNPMIPSets {
		return false
	}

	flushAndDestroy := iMgr.ioShim.Exec.Command(util.BashCommand, util.BashCommandFlag, ipsetFlushAndDestroyString)
	klog.Infof("running this command while resetting ipsets: [%s %s '%s']", util.BashCommand, util.BashCommandFlag, ipsetFlushAndDestroyString)
	output, err := flushAndDestroy.CombinedOutput()
	if err != nil {
		exitCode := -1
		stdErr := "no stdErr detected"
		var exitError utilexec.ExitError
		if ok := errors.As(err, &exitError); ok {
			exitCode = exitError.ExitStatus()
			stdErr = strings.TrimSuffix(string(output), "\n")
		}
		metrics.SendErrorLogAndMetric(util.IpsmID, "failed to flush and destroy ipsets at once. exitCode: %d. stdErr: [%v]", exitCode, stdErr)
		return false
	}
	return true
}

// this needs to be a separate function because we need to check creator contents in UTs
// named returns to appease lint
func (iMgr *IPSetManager) fileCreatorForFlushAll(ipsetListOutput []byte) (creator *ioutil.FileCreator, names []string, failedNames map[string]struct{}) {
	destroyFailureCount := 0
	creator = ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern)
	names = make([]string, 0)
	failedNames = make(map[string]struct{}, 0)
	readIndex := 0
	var line []byte
	// flush all the sets and create a list of the sets so we can destroy them
	for readIndex < len(ipsetListOutput) {
		line, readIndex = parse.Line(readIndex, ipsetListOutput)
		hashedSetName := string(line)
		if readIndex >= len(ipsetListOutput) {
			// parse.Line() will include the newline character for the end of the byte array
			hashedSetName = strings.Trim(hashedSetName, "\n")
		}
		names = append(names, hashedSetName)
		// error handlers specific to resetting ipsets
		errorHandlers := []*ioutil.LineErrorHandler{
			{
				Definition: setDoesntExistDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					klog.Infof("[RESET-IPSETS] skipping flush and upcoming destroy for set %s since the set doesn't exist", hashedSetName)
					failedNames[hashedSetName] = struct{}{}
				},
			},
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					metrics.SendErrorLogAndMetric(util.IpsmID, "[RESET-IPSETS] marking flush and upcoming destroy for set %s as a failure due to unknown error", hashedSetName)
					destroyFailureCount++
					failedNames[hashedSetName] = struct{}{}
					// TODO mark as a failure
				},
			},
		}
		sectionID := sectionID(destroySectionPrefix, hashedSetName)
		creator.AddLine(sectionID, errorHandlers, ipsetFlushFlag, hashedSetName) // flush set
	}

	return creator, names, failedNames
}

func (iMgr *IPSetManager) setsWithReferences() map[string]struct{} {
	listAllCommand := iMgr.ioShim.Exec.Command(ipsetCommand, ipsetListFlag)
	grep1 := iMgr.ioShim.Exec.Command(ioutil.Grep, ioutil.GrepBeforeFlag, referenceGrepLookBack, ioutil.GrepRegexFlag, positiveRefsRegex)
	grep2 := iMgr.ioShim.Exec.Command(ioutil.Grep, ioutil.GrepOnlyMatchingFlag, ioutil.GrepRegexFlag, azureNPMRegex)
	klog.Infof("running this command while resetting ipsets: [%s %s | %s %s %s %s %s | %s %s %s %s]", ipsetCommand, ipsetListFlag,
		ioutil.Grep, ioutil.GrepBeforeFlag, referenceGrepLookBack, ioutil.GrepRegexFlag, positiveRefsRegex,
		ioutil.Grep, ioutil.GrepOnlyMatchingFlag, ioutil.GrepRegexFlag, azureNPMRegex)
	setsWithReferencesBytes, haveRefsStill, err := ioutil.DoublePipeToGrep(listAllCommand, grep1, grep2)

	var setsWithReferences map[string]struct{}
	if haveRefsStill {
		setsWithReferences = readByteLinesToMap(setsWithReferencesBytes)
		metrics.SendErrorLogAndMetric(util.IpsmID, "error: found leaked reference counts in kernel. ipsets (max %d): %+v. err: %v",
			maxLinesToPrint, setsWithReferences, err)
	}

	return setsWithReferences
}

// named returns to appease lint
func (iMgr *IPSetManager) fileCreatorForDestroyAll(names []string, failedNames, setsWithReferences map[string]struct{}) (creator *ioutil.FileCreator, failureCount *int) {
	failureCountVal := 0
	failureCount = &failureCountVal
	creator = ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern)

	// destroy all the sets
	for _, hashedSetName := range names {
		if _, ok := failedNames[hashedSetName]; ok {
			klog.Infof("skipping destroy for set %s since it failed to flush", hashedSetName)
			continue
		}

		if _, ok := setsWithReferences[hashedSetName]; ok {
			klog.Infof("skipping destroy for set %s since it has leaked reference counts", hashedSetName)
			continue
		}

		hashedSetName := hashedSetName // to appease go lint
		errorHandlers := []*ioutil.LineErrorHandler{
			// error handlers specific to resetting ipsets
			{
				Definition: setInUseByKernelDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					metrics.SendErrorLogAndMetric(util.IpsmID, "[RESET-IPSETS] marking destroy for set %s as a failure since the set is in use by a kernel component", hashedSetName)
					failureCountVal++
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
					metrics.SendErrorLogAndMetric(util.IpsmID, "[RESET-IPSETS] marking destroy for set %s as a failure due to unknown error", hashedSetName)
					failureCountVal++
					// TODO mark the set as a failure and reconcile what iptables rule or ipset is referring to it
				},
			},
		}
		sectionID := sectionID(destroySectionPrefix, hashedSetName)
		creator.AddLine(sectionID, errorHandlers, ipsetDestroyFlag, hashedSetName) // destroy set
	}

	return creator, failureCount
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
// unused currently, but may be used later for reconciling with the kernel
func (iMgr *IPSetManager) applyIPSetsWithSaveFile() error {
	var saveFile []byte
	var saveError error
	if iMgr.dirtyCache.numSetsToAddOrUpdate() > 0 {
		saveFile, saveError = iMgr.ipsetSave()
		if saveError != nil {
			return npmerrors.SimpleErrorWrapper("ipset save failed when applying ipsets", saveError)
		}
	}
	creator := iMgr.fileCreatorForApplyWithSaveFile(maxTryCount, saveFile)
	restoreError := creator.RunCommandWithFile(ipsetCommand, ipsetRestoreFlag)
	if restoreError != nil {
		return npmerrors.SimpleErrorWrapper("ipset restore failed when applying ipsets with save file", restoreError)
	}
	return nil
}

/*
See error handling in applyIPSetsWithSaveFile().

overall format for ipset restore file:
	[creates]  (random order)
	[deletes and adds] (sets in random order, where each set has deletes first (random order), then adds (random order))
	[flushes]  (random order)
	[destroys] (random order)

example where:
- set1 and set2 will delete 1.2.3.4 and 2.3.4.5 and add 7.7.7.7 and 8.8.8.8
- set3 will be created with 1.0.0.1
- set4 and set5 will be destroyed

	restore file: [flag meanings: -F (flush), -X (destroy), -N (create), -D (delete), -A (add)]
		-N set2
		-N set3
		-N set1
		-D set2 2.3.4.5
		-D set2 1.2.3.4
		-A set2 8.8.8.8
		-A set2 7.7.7.7
		-A set3 1.0.0.1
		-D set1 1.2.3.4
		-D set1 2.3.4.5
		-A set1 7.7.7.7
		-A set1 8.8.8.8
		-F set5
		-F set4
		-X set5
		-X set4
*/
func (iMgr *IPSetManager) applyIPSets() error {
	creator := iMgr.fileCreatorForApply(maxTryCount)
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

// NOTE: duplicate code in the first step of this function and fileCreatorForApply
func (iMgr *IPSetManager) fileCreatorForApplyWithSaveFile(maxTryCount int, saveFile []byte) *ioutil.FileCreator {
	creator := ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern) // TODO make the line failure pattern into a definition constant eventually

	// 1. create all sets first so we don't try to add a member set to a list if it hasn't been created yet
	setsToAddOrUpdate := iMgr.dirtyCache.setsToAddOrUpdate()
	for prefixedName := range setsToAddOrUpdate {
		set := iMgr.setMap[prefixedName]
		iMgr.createSetForApply(creator, set)
		// NOTE: currently no logic to handle this scenario:
		// if a set in the toAddOrUpdateCache is in the kernel with the wrong type, then we'll try to create it, which will fail in the first restore call, but then be skipped in a retry
	}

	// 2. for dirty sets already in the kernel, update members (add members not in the kernel, and delete undesired members in the kernel)
	iMgr.updateDirtyKernelSets(setsToAddOrUpdate, saveFile, creator)

	// 3. for the remaining dirty sets, add their members to the kernel
	for prefixedName := range setsToAddOrUpdate {
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
	setsToDelete := iMgr.dirtyCache.setsToDelete()
	for prefixedName := range setsToDelete {
		iMgr.flushSetForApply(creator, prefixedName)
	}
	for prefixedName := range setsToDelete {
		iMgr.destroySetForApply(creator, prefixedName)
	}
	return creator
}

// NOTE: duplicate code in the first step in this function and fileCreatorForApplyWithSaveFile
func (iMgr *IPSetManager) fileCreatorForApply(maxTryCount int) *ioutil.FileCreator {
	creator := ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern) // TODO make the line failure pattern into a definition constant eventually

	// 1. create all sets first so we don't try to add a member set to a list if it hasn't been created yet
	setsToAddOrUpdate := iMgr.dirtyCache.setsToAddOrUpdate()
	for prefixedName := range setsToAddOrUpdate {
		set := iMgr.setMap[prefixedName]
		iMgr.createSetForApply(creator, set)
		// NOTE: currently no logic to handle this scenario:
		// if a set in the toAddOrUpdateCache is in the kernel with the wrong type, then we'll try to create it, which will fail in the first restore call, but then be skipped in a retry
	}

	// 2. delete/add members from dirty sets to add or update
	for prefixedName := range setsToAddOrUpdate {
		sectionID := sectionID(addOrUpdateSectionPrefix, prefixedName)
		set := iMgr.setMap[prefixedName]
		diff := iMgr.dirtyCache.memberDiff(prefixedName)
		for member := range diff.membersToDelete {
			iMgr.deleteMemberForApply(creator, set, sectionID, member)
		}
		for member := range diff.membersToAdd {
			iMgr.addMemberForApply(creator, set, sectionID, member)
		}
	}

	/*
		3. flush and destroy sets in the original delete cache

		We must perform this step after member deletions because of the following scenario:
		Suppose we want to destroy set A, which is referenced by list L. For set A to be in the toDeleteCache,
		we must have deleted the reference in list L, so list L is in the toAddOrUpdateCache. In step 2, we will delete this reference,
		but until then, set A is in use by a kernel component and can't be destroyed.
	*/
	// flush all sets first in case a set we're destroying is referenced by a list we're destroying
	setsToDelete := iMgr.dirtyCache.setsToDelete()
	for prefixedName := range setsToDelete {
		iMgr.flushSetForApply(creator, prefixedName)
	}
	for prefixedName := range setsToDelete {
		iMgr.destroySetForApply(creator, prefixedName)
	}
	return creator
}

// updates the creator (adds/deletes members) for dirty sets already in the kernel
// updates the setsToAddOrUpdate: after calling this function, the map will only consist of sets to create
// error handling principal:
// - if contract with ipset save (or grep) is breaking, salvage what we can, take a snapshot (TODO), and log the failure
// - have a background process for sending/removing snapshots intermittently
func (iMgr *IPSetManager) updateDirtyKernelSets(setsToAddOrUpdate map[string]struct{}, saveFile []byte, creator *ioutil.FileCreator) {
	// map hashed names to prefixed names
	toAddOrUpdateHashedNames := make(map[string]string)
	for prefixedName := range setsToAddOrUpdate {
		hashedName := iMgr.setMap[prefixedName].HashedName
		toAddOrUpdateHashedNames[hashedName] = prefixedName
	}

	// in each iteration, read a create line and any ensuing add lines
	readIndex := 0
	var line []byte
	if readIndex < len(saveFile) {
		line, readIndex = parse.Line(readIndex, saveFile)
		if !hasPrefix(line, createStringWithSpace) {
			metrics.SendErrorLogAndMetric(util.IpsmID, "expected a create line in ipset save file, but got the following line: %s", string(line))
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
		delete(setsToAddOrUpdate, prefixedName)
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
				metrics.SendErrorLogAndMetric(util.IpsmID, "expected an add line, but got the following line: %s", string(line))
				// TODO send error snapshot
				line, readIndex = nextCreateLine(readIndex, saveFile)
				break
			}
			lineAfterAdd := string(line[len(addStringWithSpace):])
			spaceSplitLineAfterAdd := strings.Split(lineAfterAdd, space)
			parent := spaceSplitLineAfterAdd[0]
			if len(spaceSplitLineAfterAdd) != 2 || parent != hashedName {
				metrics.SendErrorLogAndMetric(util.IpsmID, "expected an add line for set %s in ipset save file, but got the following line: %s", hashedName, string(line))
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
			metrics.SendErrorLogAndMetric(util.IpsmID, "expected to find a ListSet but have the line: %s", lineString)
			return true
		}
	case ipsetNetHashString:
		if set.Kind != HashSet || set.Type == NamedPorts {
			lineString := fmt.Sprintf("create %s %s", set.HashedName, strings.Join(restOfSpaceSplitCreateLine, " "))
			metrics.SendErrorLogAndMetric(util.IpsmID, "expected to find a non-NamedPorts HashSet but have the following line: %s", lineString)
			return true
		}
	case ipsetIPPortHashString:
		if set.Type != NamedPorts {
			lineString := fmt.Sprintf("create %s %s", set.HashedName, strings.Join(restOfSpaceSplitCreateLine, " "))
			metrics.SendErrorLogAndMetric(util.IpsmID, "expected to find a NamedPorts set but have the following line: %s", lineString)
			return true
		}
	default:
		metrics.SendErrorLogAndMetric(util.IpsmID, "unknown type string [%s] in line: %s", typeString, strings.Join(restOfSpaceSplitCreateLine, " "))
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
				metrics.SendErrorLogAndMetric(util.IpsmID, "skipping flush and upcoming destroy for set %s due to unknown error", prefixedName)
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
				metrics.SendErrorLogAndMetric(util.IpsmID, "skipping destroy line for set %s since the set is in use by a kernel component", prefixedName)
				// TODO mark the set as a failure and reconcile what iptables rule or ipset is referring to it
			},
		},
		{
			Definition: ioutil.AlwaysMatchDefinition,
			Method:     ioutil.Continue,
			Callback: func() {
				metrics.SendErrorLogAndMetric(util.IpsmID, "skipping destroy line for set %s due to unknown error", prefixedName)
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
				metrics.SendErrorLogAndMetric(util.IpsmID, "skipping create and any following adds/deletes for set %s since the set already exists with different specs", prefixedName)
				// TODO mark the set as a failure and handle this
			},
		},
		{
			Definition: ioutil.AlwaysMatchDefinition,
			Method:     ioutil.ContinueAndAbortSection,
			Callback: func() {
				metrics.SendErrorLogAndMetric(util.IpsmID, "skipping create and any following adds/deletes for set %s due to unknown error", prefixedName)
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
				metrics.SendErrorLogAndMetric(util.IpsmID, "skipping delete line for set %s due to unknown error", set.Name)
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
					metrics.SendErrorLogAndMetric(util.IpsmID, "skipping add of %s to list %s since the member doesn't exist", member, set.Name)
					// TODO reconcile
				},
			},
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					metrics.SendErrorLogAndMetric(util.IpsmID, "skipping add of %s to list %s due to unknown error", member, set.Name)
				},
			},
		}
	} else {
		errorHandlers = []*ioutil.LineErrorHandler{
			{
				Definition: ioutil.AlwaysMatchDefinition,
				Method:     ioutil.Continue,
				Callback: func() {
					metrics.SendErrorLogAndMetric(util.IpsmID, "skipping add line for hash set %s due to unknown error", set.Name)
				},
			},
		}
	}
	creator.AddLine(sectionID, errorHandlers, ipsetAddFlag, set.HashedName, member) // add member
}

func sectionID(prefix, prefixedName string) string {
	return fmt.Sprintf("%s-%s", prefix, prefixedName)
}

func readByteLinesToMap(output []byte) map[string]struct{} {
	readIndex := 0
	var line []byte
	lines := make(map[string]struct{})
	for readIndex < len(output) {
		line, readIndex = parse.Line(readIndex, output)
		hashedSetName := strings.Trim(string(line), "\n")
		lines[hashedSetName] = struct{}{}
		if len(lines) > maxLinesToPrint {
			break
		}
	}
	return lines
}
