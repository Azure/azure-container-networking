package errors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-container-networking/npm/util"
)

var (
	// ErrSrcNotSpecified thrown during NPM debug cli mode when the source packet is not specified
	ErrSrcNotSpecified = errors.New("source not specified")

	// ErrDstNotSpecified thrown during NPM debug cli mode when the source packet is not specified
	ErrDstNotSpecified = errors.New("destination not specified")
)

/*
✅ | where Raw !contains "Set cannot be destroyed: it is in use by a kernel component" // Exit status 1
✅ | where Raw !contains "Elem separator in" // Error: There was an error running command: [ipset -A -exist azure-npm-527074092 10.104.7.252,3000] Stderr: [exit status 1, ipset v7.5: Syntax error: Elem separator in 10.104.7.252,3000, but settype hash:net supports none.]
✅ | where Raw !contains "The set with the given name does not exist" // Exit status 1
❌ | where Raw !contains "TSet cannot be created: set with the same name already exists" // Exit status 1
❌| where Raw !contains "failed to create ipset rules" // Exit status 1
✅ | where Raw !contains "Second element is missing from" //Error: There was an error running command: [ipset -A -exist azure-npm-4041682038 172.16.48.55] Stderr: [exit status 1, ipset v7.5: Syntax error: Second element is missing from 172.16.48.55.]
❌| where Raw !contains "Error: failed to create ipset."
✅ | where Raw !contains "Missing second mandatory argument to command del" //Error: There was an error running command: [ipset -D -exist azure-npm-2064349730] Stderr: [exit status 2, ipset v7.5: Missing second mandatory argument to command del Try `ipset help' for more information.]
✅ | where Raw !contains "Kernel error received: maximal number of sets reached" //Error: There was an error running command: [ipset -N -exist azure-npm-804639716 nethash] Stderr: [exit status 1, ipset v7.5: Kernel error received: maximal number of sets reached, cannot create more.]
❌ | where Raw !contains "failed to create ipset list"
❌ | where Raw !contains "set with the same name already exists"
| where Raw !contains "failed to delete ipset entry"
| where Raw !contains "Set to be added/deleted/tested as element does not exist"
| where (Raw !contains "ipset list with" and Raw !contains "not found")
| where Raw !contains "cannot parse azure: resolving to IPv4 address failed" //Error: There was an error running command: [ipset -A -exist azure-npm-2711086373 azure-npm-3224564478] Stderr: [exit status 1, ipset v7.5: Syntax error: cannot parse azure: resolving to IPv4 address failed]
| where Raw !contains "Sets with list:set type cannot be added to the set" //Error: There was an error running command: [ipset -A -exist azure-npm-530439631 azure-npm-2711086373] Stderr: [exit status 1, ipset v7.5: Sets with list:set type cannot be added to the set.]
| where Raw !contains "failed to delete ipset" // Different from the one above
| where Raw !contains " The value of the CIDR parameter of the IP address" //Error: There was an error running command: [ipset -A -exist azure-npm-3142322720 10.2.1.227/0] Stderr: [exit status 1, ipset v7.5: The value of the CIDR parameter of the IP address is invalid]
| where (Raw !contains "Syntax error:" and Raw !contains "is out of range 0-32") // IPV6 addr Error: There was an error running command: [ipset -A -exist azure-npm-1389121076 fe80::/64] Stderr: [exit status 1, ipset v7.5: Syntax error: '64' is out of range 0-32]
| where Raw !contains "exit status 253" // need to understand
| where Raw !contains "Operation not permitted" // Most probably from UTs
*/

// Error labels for ipsetmanager
const (
	CreateIPSet             = "CreateIPSet"
	AppendIPSet             = "AppendIPSet"
	DeleteIPSet             = "DeleteIPSet"
	DestroyIPSet            = "DestroyIPSet"
	TestIPSet               = "TestIPSet"
	IPSetIntersection       = "IPSetIntersection"
	AddPolicy               = "AddNetworkPolicy"
	GetSelectorReference    = "GetSelectorReference"
	AddSelectorReference    = "AddSelectorReference"
	DeleteSelectorReference = "DeleteSelectorReference"
	AddNetPolReference      = "AddNetPolReference"
	DeleteNetPolReference   = "DeleteNetPolReference"
)

// Error codes for ipsetmanager
const (
	SetCannotBeDestroyedInUseByKernelComponent = 1
	ElemSeperatorNotSupported                  = 2
	SetWithGivenNameDoesNotExist               = 3
	SecondElementIsMissing                     = 4
	MissingSecondMandatoryArgument             = 5
	MaximalNumberOfSetsReached                 = 6
	IPSetWithGivenNameAlreadyExists            = 7
	SetToBeAddedDeletedTestedDoesNotExist      = 8
	Unknown                                    = 999
)

var (
	ipseterrs = map[int]npmErrorDefinition{
		SetCannotBeDestroyedInUseByKernelComponent: {"Set cannot be destroyed: it is in use by a kernel component", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		ElemSeperatorNotSupported:                  {"Syntax error: Elem separator", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		SetWithGivenNameDoesNotExist:               {"The set with the given name does not exist", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		SecondElementIsMissing:                     {"Second element is missing from", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		MissingSecondMandatoryArgument:             {"Missing second mandatory argument to command", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		MaximalNumberOfSetsReached:                 {"Kernel error received: maximal number of sets reached", npmErrorRetrySettings{Create: false, Append: false, Delete: false}}, // no ut
		IPSetWithGivenNameAlreadyExists:            {"Set cannot be created: set with the same name already exists", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		SetToBeAddedDeletedTestedDoesNotExist:      {"Set to be added/deleted/tested as element does not exist", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
		Unknown:                                    {"Unknown error", npmErrorRetrySettings{Create: false, Append: false, Delete: false}},
	}

	operationstrings = map[string]string{
		util.IpsetAppendFlag:   AppendIPSet,
		util.IpsetCreationFlag: CreateIPSet,
		util.IpsetDeletionFlag: DeleteIPSet,
		util.IpsetDestroyFlag:  DestroyIPSet,
		util.IpsetTestFlag:     TestIPSet,
	}
)

func ConvertToNPMError(operationFlag string, err error, cmd []string) *NPMError {
	for code, errDefinition := range ipseterrs {
		errstr := err.Error()
		if strings.Contains(errstr, errDefinition.description) {
			return &NPMError{
				OperationAction: operationstrings[operationFlag],
				IsRetriable:     errDefinition.IsRetriable(operationFlag),
				FullCmd:         cmd,
				ErrID:           code,
				Err:             err,
			}
		}
	}

	return &NPMError{
		OperationAction: operationstrings[operationFlag],
		IsRetriable:     false,
		FullCmd:         cmd,
		ErrID:           Unknown,
		Err:             err,
	}
}

func Errorf(operation string, isRetriable bool, errstring string) *NPMError {
	return &NPMError{
		OperationAction: operation,
		IsRetriable:     false,
		FullCmd:         []string{},
		ErrID:           Unknown,
		Err:             fmt.Errorf("%s", errstring),
	}
}

func Error(operation string, isRetriable bool, err error) *NPMError {
	return &NPMError{
		OperationAction: operation,
		IsRetriable:     false,
		FullCmd:         []string{},
		ErrID:           Unknown,
		Err:             err,
	}
}

type NPMError struct {
	OperationAction string
	IsRetriable     bool
	FullCmd         []string
	ErrID           int
	Err             error
}

func (n *npmErrorDefinition) IsRetriable(operationAction string) bool {
	switch operationAction {
	case CreateIPSet:
		return n.isRetriable.Create
	case DeleteIPSet:
		return n.isRetriable.Delete
	case AppendIPSet:
		return n.isRetriable.Append
	}
	return false
}

type npmErrorDefinition struct {
	description string
	isRetriable npmErrorRetrySettings
}

type npmErrorRetrySettings struct {
	Create bool
	Append bool
	Delete bool
}

func (n *NPMError) Error() string {
	return fmt.Sprintf("Operation [%s] failed with error code [%v], full cmd %v, full error %v", n.OperationAction, n.ErrID, n.FullCmd, n.Err)
}
