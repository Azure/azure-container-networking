package ipsets

import (
	"fmt"

	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ioutil"
	"github.com/Azure/azure-container-networking/npm/util"
)

const (
	maxTryCount                    = 1
	deletionPrefix                 = "delete"
	creationPrefix                 = "create"
	ipsetRestoreLineFailurePattern = "Error in line (\\d+):"
	setAlreadyExistsPattern        = "Set cannot be created: set with the same name already exists"
	setDoesntExistPattern          = "The set with the given name does not exist"
	setInUseByKernelPattern        = "Set cannot be destroyed: it is in use by a kernel component"
	memberSetDoesntExist           = "Set to be added/deleted/tested as element does not exist"
)

func (iMgr *IPSetManager) resetIPSets() error {
	// called on failure or when NPM is created
	// so no ipset cache. need to use ipset list like in ipsm.go

	// create restore file that flushes all sets, then deletes all sets
	// technically don't need to flush a hashset

	return nil
}

// don't need networkID
func (iMgr *IPSetManager) applyIPSets() error {
	toDeleteSetNames := convertAndDeleteCache(iMgr.toDeleteCache)
	toAddOrUpdateSetNames := convertAndDeleteCache(iMgr.toAddOrUpdateCache)
	creator := iMgr.getFileCreator(maxTryCount, toDeleteSetNames, toAddOrUpdateSetNames)
	err := creator.RunCommandWithFile(util.Ipset, util.IpsetRestoreFlag)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

func convertAndDeleteCache(cache map[string]struct{}) []string {
	result := make([]string, len(cache))
	i := 0
	for setName := range cache {
		result[i] = setName
		delete(cache, setName)
		i++
	}
	return result
}

// getFileCreator encodes an ipset restore file with error handling.
// We use slices instead of maps so we can have determinstic behavior for
// unit tests on the file creator i.e. check file contents before and after error handling.
// Without slices, we could do unit tests on certain segments of the file,
// but things would get complicated for checking error handling.
// We can't escape the nondeterministic behavior of adding members,
// but we can handle this in UTs with sorting.
func (iMgr *IPSetManager) getFileCreator(maxTryCount int, toDeleteSetNames, toAddOrUpdateSetNames []string) *ioutil.FileCreator {
	creator := ioutil.NewFileCreator(iMgr.ioShim, maxTryCount, ipsetRestoreLineFailurePattern)
	// creator.AddErrorToRetryOn(ioutil.NewErrorDefinition("something")) // TODO add file-level errors?
	iMgr.handleDeletions(creator, toDeleteSetNames)
	iMgr.handleAddOrUpdates(creator, toAddOrUpdateSetNames)
	return creator
}

func (iMgr *IPSetManager) handleDeletions(creator *ioutil.FileCreator, setNames []string) {
	// flush all first so we don't try to delete an ipset referenced by a list we're deleting too
	// error handling:
	// - abort the flush and delete call for a set if the set doesn't exist
	// - if the set is in use by a kernel component, then skip the delete and mark it as a failure
	for _, setName := range setNames {
		setName := setName // to appease golint complaints about function literal
		errorHandlers := []*ioutil.LineErrorHandler{
			{
				Definition: ioutil.NewErrorDefinition(setDoesntExistPattern),
				Method:     ioutil.AbortSection,
				Callback: func() {
					// no action needed since we expect that it's gone after applyIPSets()
					log.Logf("was going to delete set %s but it doesn't exist", setName)
				},
			},
		}
		sectionID := getSectionID(deletionPrefix, setName)
		hashedSetName := util.GetHashedName(setName)
		creator.AddLine(sectionID, errorHandlers, util.IpsetFlushFlag, hashedSetName) // flush set
	}

	for _, setName := range setNames {
		setName := setName // to appease golint complaints about function literal
		errorHandlers := []*ioutil.LineErrorHandler{
			{
				Definition: ioutil.NewErrorDefinition(setInUseByKernelPattern),
				Method:     ioutil.SkipLine,
				Callback: func() {
					log.Errorf("was going to delete set %s but it is in use by a kernel component", setName)
					// TODO mark the set as a failure and reconcile what iptables rule or ipset is referring to it
				},
			},
		}
		sectionID := getSectionID(deletionPrefix, setName)
		hashedSetName := util.GetHashedName(setName)
		creator.AddLine(sectionID, errorHandlers, util.IpsetDestroyFlag, hashedSetName) // destroy set
	}
}

func (iMgr *IPSetManager) handleAddOrUpdates(creator *ioutil.FileCreator, setNames []string) {
	// create all sets first
	// error handling:
	// - abort the create, flush, and add calls if create doesn't work
	//   Won't abort adding the set to a list. Will need another retry to handle that
	//   TODO change this behavior?
	for _, setName := range setNames {
		set := iMgr.setMap[setName]

		methodFlag := util.IpsetNetHashFlag
		if set.Kind == ListSet {
			methodFlag = util.IpsetSetListFlag
		} else if set.Type == NamedPorts {
			methodFlag = util.IpsetIPPortHashFlag
		}

		specs := []string{util.IpsetCreationFlag, set.HashedName, util.IpsetExistFlag, methodFlag}
		if set.Type == CIDRBlocks {
			specs = append(specs, util.IpsetMaxelemName, util.IpsetMaxelemNum)
		}

		setName := setName // to appease golint complaints about function literal
		errorHandlers := []*ioutil.LineErrorHandler{
			{
				Definition: ioutil.NewErrorDefinition(setAlreadyExistsPattern),
				Method:     ioutil.AbortSection,
				Callback: func() {
					log.Errorf("was going to add/update set %s but couldn't create the set", setName)
					// TODO mark the set as a failure and handle this
				},
			},
		}
		sectionID := getSectionID(creationPrefix, setName)
		creator.AddLine(sectionID, errorHandlers, specs...) // create set
	}

	// flush and add all IPs/members for each set
	// error handling:
	// - if a member set can't be added to a list because it doesn't exist, then skip the add and mark it as a failure
	for _, setName := range setNames {
		set := iMgr.setMap[setName]
		sectionID := getSectionID(creationPrefix, setName)
		creator.AddLine(sectionID, nil, util.IpsetFlushFlag, set.HashedName) // flush set (no error handler needed)

		if set.Kind == HashSet {
			for ip := range set.IPPodKey {
				// TODO add error handler?
				creator.AddLine(sectionID, nil, util.IpsetAppendFlag, set.HashedName, ip) // add IP
			}
		} else {
			setName := setName // to appease golint complaints about function literal
			for _, member := range set.MemberIPSets {
				memberName := member.Name // to appease golint complaints about function literal
				errorHandlers := []*ioutil.LineErrorHandler{
					{
						Definition: ioutil.NewErrorDefinition(memberSetDoesntExist),
						Method:     ioutil.SkipLine,
						Callback: func() {
							log.Errorf("was going to add member set %s to list %s, but the member doesn't exist", memberName, setName)
							// TODO handle error
						},
					},
				}
				creator.AddLine(sectionID, errorHandlers, util.IpsetAppendFlag, set.HashedName, member.HashedName) // add member
			}
		}
	}
}

func getSectionID(prefix, setName string) string {
	return fmt.Sprintf("%s-%s", prefix, setName)
}
