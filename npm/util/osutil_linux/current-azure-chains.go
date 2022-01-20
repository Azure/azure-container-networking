//go:build !windows
// +build !windows

package linuxutil

import (
	"errors"
	"strings"

	"github.com/Azure/azure-container-networking/npm/util"
	npmerrors "github.com/Azure/azure-container-networking/npm/util/errors"
	"k8s.io/klog"
	utilexec "k8s.io/utils/exec"
)

const (
	azureChainGrepPattern   string = "Chain AZURE-NPM"
	minAzureChainNameLength int    = len("AZURE-NPM")
	// the minimum number of sections when "Chain NAME (1 references)" is split on spaces (" ")
	minSpacedSectionsForChainLine int = 2
)

var errInvalidGrepResult = errors.New("unexpectedly got no lines while grepping for current Azure chains")

func AllCurrentAzureChains(exec utilexec.Interface, defaultlockWaitTimeInSeconds string) (map[string]struct{}, error) {
	iptablesListCommand := exec.Command(util.Iptables,
		util.IptablesWaitFlag, defaultlockWaitTimeInSeconds, util.IptablesTableFlag, util.IptablesFilterTable,
		util.IptablesNumericFlag, util.IptablesListFlag,
	)
	grepCommand := exec.Command(Grep, azureChainGrepPattern)
	searchResults, gotMatches, err := PipeCommandToGrep(iptablesListCommand, grepCommand)
	if err != nil {
		return nil, npmerrors.SimpleErrorWrapper("failed to get policy chain names", err)
	}
	if !gotMatches {
		return nil, nil
	}
	lines := strings.Split(string(searchResults), "\n")
	if len(lines) == 1 && lines[0] == "" {
		// this should never happen: gotMatches is true, but there is no content in the searchResults
		return nil, errInvalidGrepResult
	}
	lastIndex := len(lines) - 1
	lastLine := lines[lastIndex]
	if lastLine == "" {
		// remove the last empty line (since each line ends with a newline)
		lines = lines[:lastIndex] // this line doesn't impact the array that the slice references
	} else {
		klog.Errorf(`while grepping for current Azure chains, expected last line to end in "" but got [%s]. full grep output: [%s]`, lastLine, string(searchResults))
	}
	chainNames := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		// line of the form "Chain NAME (1 references)"
		spaceSeparatedLine := strings.Split(line, " ")
		if len(spaceSeparatedLine) < minSpacedSectionsForChainLine || len(spaceSeparatedLine[1]) < minAzureChainNameLength {
			klog.Errorf("while grepping for current Azure chains, got unexpected line [%s] for all current azure chains. full grep output: [%s]", line, string(searchResults))
		} else {
			chainNames[spaceSeparatedLine[1]] = struct{}{}
		}
	}
	return chainNames, nil
}
