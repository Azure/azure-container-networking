package ipsets

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/metrics/promutil"
	dptestutils "github.com/Azure/azure-container-networking/npm/pkg/dataplane/testutils"
	"github.com/Azure/azure-container-networking/npm/util"
	"github.com/Azure/azure-container-networking/npm/util/ioutil"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/require"
)

const (
	saveResult = "create test-list1 list:set size 8\nadd test-list1 test-list2"

	createNethashFormat  = "create %s hash:net family inet hashsize 1024 maxelem 65536"
	createPorthashFormat = "create %s hash:ip,port family inet hashsize 1024 maxelem 65536"
	createListFormat     = "create %s list:set size 8"
)

var (
	// need to be sorted
	resetIPSetsNames            = []string{"azure-npm-123456", "azure-npm-777777", "azure-npm-987654"}
	resetIPSetsListOutputString = strings.Join(resetIPSetsNames, "\n") + "\n"
	resetIPSetsListOutput       = []byte(resetIPSetsListOutputString)
	otherIPSetsListOutput       = "azure-npm-123456\n"
)

// TODO test that a reconcile list is updated for all the TestFailure UTs
// TODO same exact TestFailure UTs for unknown errors

func TestApplyIPSets(t *testing.T) {
	type args struct {
		toAddUpdateSets []*IPSetMetadata
		toDeleteSets    []*IPSetMetadata
		commandError    bool
	}
	tests := []struct {
		name              string
		args              args
		expectedExecCount int
		wantErr           bool
	}{
		{
			name: "nothing to update",
			args: args{
				toAddUpdateSets: nil,
				toDeleteSets:    nil,
				commandError:    false,
			},
			expectedExecCount: 0,
			wantErr:           false,
		},
		{
			name: "success with just add",
			args: args{
				toAddUpdateSets: []*IPSetMetadata{namespaceSet},
				toDeleteSets:    nil,
				commandError:    false,
			},
			expectedExecCount: 1,
			wantErr:           false,
		},
		{
			name: "success with just delete",
			args: args{
				toAddUpdateSets: []*IPSetMetadata{namespaceSet},
				toDeleteSets:    nil,
				commandError:    false,
			},
			expectedExecCount: 1,
			wantErr:           false,
		},
		{
			name: "success with add and delete",
			args: args{
				toAddUpdateSets: []*IPSetMetadata{namespaceSet},
				toDeleteSets:    []*IPSetMetadata{keyLabelOfPodSet},
				commandError:    false,
			},
			expectedExecCount: 1,
			wantErr:           false,
		},
		{
			name: "apply error",
			args: args{
				toAddUpdateSets: []*IPSetMetadata{namespaceSet},
				toDeleteSets:    []*IPSetMetadata{keyLabelOfPodSet},
				commandError:    true,
			},
			expectedExecCount: 1,
			wantErr:           true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			metrics.ReinitializeAll()
			calls := GetApplyIPSetsTestCalls(tt.args.toAddUpdateSets, tt.args.toDeleteSets)
			if tt.args.commandError {
				// add an error to the last call (the ipset-restore call)
				// this would potentially cause problems if we used pointers to the TestCmds
				require.Greater(t, len(calls), 0)
				calls[len(calls)-1].ExitCode = 1
				// then add errors as many times as we retry
				for i := 1; i < maxTryCount; i++ {
					calls = append(calls, testutils.TestCmd{Cmd: ipsetRestoreStringSlice, ExitCode: 1})
				}
			}
			ioShim := common.NewMockIOShim(calls)
			defer ioShim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)
			iMgr.CreateIPSets(tt.args.toAddUpdateSets)
			for _, set := range tt.args.toDeleteSets {
				iMgr.dirtyCache.destroy(NewIPSet(set))
			}
			err := iMgr.ApplyIPSets()

			// cache behavior is currently undefined if there's an apply error
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				cache := make([]setMembers, 0)
				for _, set := range tt.args.toAddUpdateSets {
					cache = append(cache, setMembers{metadata: set, members: nil})
				}
				assertExpectedInfo(t, iMgr, &expectedInfo{
					mainCache:        cache,
					toAddUpdateCache: nil,
					toDeleteCache:    nil,
					setsForKernel:    nil,
				})
			}

			execCount, err := metrics.GetIPSetExecCount()
			promutil.NotifyIfErrors(t, err)
			require.Equal(t, tt.expectedExecCount, execCount)
		})
	}
}

func TestNextCreateLine(t *testing.T) {
	createLine := "create test-list1 list:set size 8"
	addLine := "add test-set1 1.2.3.4"
	createLineWithNewline := createLine + "\n"
	addLineWithNewline := addLine + "\n"
	tests := []struct {
		name              string
		lines             []string
		expectedReadIndex int
		expectedLine      []byte
	}{
		// parse.Line() will omit the newline at the end of the line unless it's the last line
		{
			name:              "empty save file",
			lines:             []string{},
			expectedReadIndex: 0,
			expectedLine:      nil,
		},
		{
			name:              "no creates",
			lines:             []string{addLineWithNewline},
			expectedReadIndex: len(addLineWithNewline),
			expectedLine:      []byte(addLineWithNewline),
		},
		{
			name:              "start with create",
			lines:             []string{createLine, addLineWithNewline},
			expectedReadIndex: len(createLineWithNewline),
			expectedLine:      []byte(createLine),
		},
		{
			name:              "create after adds",
			lines:             []string{addLine, addLine, createLineWithNewline},
			expectedReadIndex: 2*len(addLine+"\n") + len(createLine+"\n"),
			expectedLine:      []byte(createLineWithNewline),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			saveFile := []byte(strings.Join(tt.lines, "\n"))
			line, readIndex := nextCreateLine(0, saveFile)
			require.Equal(t, tt.expectedReadIndex, readIndex)
			require.Equal(t, tt.expectedLine, line)
		})
	}
}

func TestDestroyNPMIPSetsCreatorSuccess(t *testing.T) {
	calls := []testutils.TestCmd{fakeRestoreSuccessCommand, fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)
	creator, names, failedNames := iMgr.fileCreatorForFlushAll(resetIPSetsListOutput)
	actualLines := strings.Split(creator.ToString(), "\n")
	expectedLines := []string{
		"-F azure-npm-123456",
		"-F azure-npm-777777",
		"-F azure-npm-987654",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
	sort.Strings(names)
	require.Equal(t, resetIPSetsNames, names, "got unexpected ipset names")
	wasModified, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.False(t, wasModified, "got unexpected flush modify flag")
	require.NoError(t, err, "got unexpected flush error")
	require.Len(t, failedNames, 0, "got unexpected flush failure count")

	creator, destroyFailureCount := iMgr.fileCreatorForDestroyAll(names, failedNames, nil)
	actualLines = strings.Split(creator.ToString(), "\n")
	expectedLines = []string{
		"-X azure-npm-123456",
		"-X azure-npm-777777",
		"-X azure-npm-987654",
		"",
	}
	dptestutils.AssertEqualLines(t, expectedLines, actualLines)
	wasModified, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.False(t, wasModified, "got unexpected destroy modified flag")
	require.NoError(t, err, "got unexpected destroy error")
	require.Equal(t, 0, *destroyFailureCount, "got unexpected destroy failure count")
}

func TestDestroyNPMIPSetsCreatorErrorHandling(t *testing.T) {
	/*
		original lines:

		"-F azure-npm-123456",
		"-F azure-npm-777777",
		"-F azure-npm-987654",
	*/
	tests := []struct {
		name                   string
		calls                  []testutils.TestCmd
		expectedFlushFailure   bool
		expectedFlushLines     []string
		expectedFailedNames    map[string]struct{}
		expectedDestroyFailure bool
		expectedDestroyLines   []string
		expectedFailureCount   int
		setsWithReferences     map[string]struct{}
	}{
		{
			name: "set doesn't exist on flush",
			calls: []testutils.TestCmd{
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: The set with the given name does not exist",
					ExitCode: 1,
				},
				fakeRestoreSuccessCommand,
			},
			expectedFlushFailure: true,
			expectedFlushLines: []string{
				"-F azure-npm-987654",
				"",
			},
			expectedFailedNames: map[string]struct{}{
				"azure-npm-777777": {},
			},
			expectedDestroyFailure: false,
			expectedDestroyLines: []string{
				"-X azure-npm-123456",
				"-X azure-npm-987654",
				"",
			},
			expectedFailureCount: 0,
		},
		{
			name: "some other error on flush",
			calls: []testutils.TestCmd{
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: for some other error",
					ExitCode: 1,
				},
				fakeRestoreSuccessCommand,
			},
			expectedFlushFailure: true,
			expectedFlushLines: []string{
				"-F azure-npm-987654",
				"",
			},
			expectedFailedNames: map[string]struct{}{
				"azure-npm-777777": {},
			},
			expectedDestroyFailure: false,
			expectedDestroyLines: []string{
				"-X azure-npm-123456",
				"-X azure-npm-987654",
				"",
			},
			expectedFailureCount: 0,
		},
		{
			name: "set doesn't exist on destroy",
			calls: []testutils.TestCmd{
				fakeRestoreSuccessCommand,
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: The set with the given name does not exist",
					ExitCode: 1,
				},
			},
			expectedFlushFailure:   false,
			expectedDestroyFailure: true,
			expectedDestroyLines: []string{
				"-X azure-npm-987654",
				"",
			},
			expectedFailureCount: 0,
		},
		{
			name: "some other error on destroy",
			calls: []testutils.TestCmd{
				fakeRestoreSuccessCommand,
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: some other error",
					ExitCode: 1,
				},
			},
			expectedFlushFailure:   false,
			expectedDestroyFailure: true,
			expectedDestroyLines: []string{
				"-X azure-npm-987654",
				"",
			},
			expectedFailureCount: 1,
		},
		{
			name: "set with references",
			calls: []testutils.TestCmd{
				fakeRestoreSuccessCommand,
				fakeRestoreSuccessCommand,
			},
			expectedFlushFailure:   false,
			expectedDestroyFailure: false,
			expectedDestroyLines: []string{
				"-X azure-npm-123456",
				"-X azure-npm-987654",
				"",
			},
			expectedFailureCount: 0,
			setsWithReferences: map[string]struct{}{
				"azure-npm-777777": {},
			},
		},
		{
			name: "all sets with references",
			calls: []testutils.TestCmd{
				fakeRestoreSuccessCommand,
			},
			expectedFlushFailure:   false,
			expectedDestroyFailure: false,
			expectedDestroyLines: []string{
				"",
			},
			expectedFailureCount: 0,
			setsWithReferences: map[string]struct{}{
				"azure-npm-123456": {},
				"azure-npm-777777": {},
				"azure-npm-987654": {},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)
			creator, names, failedNames := iMgr.fileCreatorForFlushAll(resetIPSetsListOutput)

			sort.Strings(names)
			require.Equal(t, resetIPSetsNames, names, "got unexpected ipset names")
			wasModified, err := creator.RunCommandOnceWithFile("ipset", "restore")
			if tt.expectedFlushFailure {
				require.True(t, wasModified, "got unexpected flush modify flag")
				require.Error(t, err, "got unexpected flush success")
				fmt.Println("err:", err.Error())
				require.Equal(t, failedNames, tt.expectedFailedNames)
				actualLines := strings.Split(creator.ToString(), "\n")
				dptestutils.AssertEqualLines(t, tt.expectedFlushLines, actualLines)
			} else {
				require.False(t, wasModified, "got unexpected flush modify flag")
				require.NoError(t, err, "got unexpected flush error")
				require.Len(t, failedNames, 0)
			}

			creator, destroyFailureCount := iMgr.fileCreatorForDestroyAll(names, failedNames, tt.setsWithReferences)
			wasModified, err = creator.RunCommandOnceWithFile("ipset", "restore")
			if tt.expectedDestroyFailure {
				require.True(t, wasModified, "got unexpected destroy modify flag")
				require.Error(t, err, "got unexpected destroy success")
				fmt.Println("err:", err.Error())
				require.Equal(t, tt.expectedFailureCount, *destroyFailureCount, "got unexpected failure count")
				actualLines := strings.Split(creator.ToString(), "\n")
				dptestutils.AssertEqualLines(t, tt.expectedDestroyLines, actualLines)
			} else {
				require.False(t, wasModified, "got unexpected destroy modify flag")
				require.NoError(t, err, "got unexpected destroy error")
				require.Equal(t, 0, *destroyFailureCount, "got unexpected failure count")
				actualLines := strings.Split(creator.ToString(), "\n")
				dptestutils.AssertEqualLines(t, tt.expectedDestroyLines, actualLines)
			}
		})
	}
}

func TestDestroyNPMIPSets(t *testing.T) {
	tests := []struct {
		name    string
		calls   []testutils.TestCmd
		wantErr bool
	}{
		{
			name: "non-azure sets don't exist",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 1}, // grep didn't find anything
				{Cmd: []string{"bash", "-c", "ipset flush && ipset destroy"}},
			},
			wantErr: false,
		},
		{
			name: "ignore failure when looking for non-azure sets",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 1}, // grep didn't find anything
				{Cmd: []string{"bash", "-c", "ipset flush && ipset destroy"}, ExitCode: 1, Stdout: "some error here"},
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, ExitCode: 1},
			},
			wantErr: false,
		},
		{
			name: "success with no results from grep",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, ExitCode: 1},
			},
			wantErr: false,
		},
		{
			name: "successfully delete sets",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, ExitCode: 1},
				fakeRestoreSuccessCommand,
			},
			wantErr: false,
		},
		{
			name: "grep error",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true, HasStartError: true, ExitCode: 1},
				{Cmd: []string{"grep", "azure-npm-"}},
			},
			wantErr: true,
		},
		{
			name: "restore error from max flush tries",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
			},
			wantErr: true,
		},
		{
			name: "restore error from max destroy tries",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
				{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
			},
			wantErr: true,
		},
		{
			name: "success despite all sets having references",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, Stdout: resetIPSetsListOutputString},
			},
			wantErr: false,
		},
		{
			name: "success despite one set having references",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, Stdout: otherIPSetsListOutput},
				fakeRestoreSuccessCommand,
			},
			wantErr: false,
		},
		{
			name: "successfully restore, but fail to flush/destroy 1 set since the set doesn't exist when flushing",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: The set with the given name does not exist",
					ExitCode: 1,
				},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, ExitCode: 1},
				fakeRestoreSuccessCommand,
			},
			wantErr: false,
		},
		{
			name: "successfully restore, but fail to flush/destroy 1 set due to other flush error",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: for some other error",
					ExitCode: 1,
				},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, ExitCode: 1},
				fakeRestoreSuccessCommand,
			},
			wantErr: false,
		},
		{
			name: "successfully restore, but fail to destroy 1 set since the set doesn't exist when destroying",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, ExitCode: 1},
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: The set with the given name does not exist",
					ExitCode: 1,
				},
				fakeRestoreSuccessCommand,
			},
			wantErr: false,
		},
		{
			name: "successfully restore, but fail to destroy 1 set due to other destroy error",
			calls: []testutils.TestCmd{
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
				{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true},
				{Cmd: []string{"grep", "azure-npm-"}, Stdout: resetIPSetsListOutputString},
				fakeRestoreSuccessCommand,
				{Cmd: []string{"ipset", "list"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-B", "5", "-P", "References: [1-9]"}, PipedToCommand: true},
				{Cmd: []string{"grep", "-o", "-P", "azure-npm-\\d+"}, ExitCode: 1},
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   "Error in line 2: for some other error",
					ExitCode: 1,
				},
				fakeRestoreSuccessCommand,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ioshim := common.NewMockIOShim(tt.calls)
			defer ioshim.VerifyCalls(t, tt.calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)
			err := iMgr.resetIPSets()
			if tt.wantErr {
				require.Error(t, err)
				fmt.Println("got correct error:", err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// identical to TestResetIPSets in ipsetmanager_test.go except an error occurs
// makes sure that the cache and metrics are reset despite error
func TestResetIPSetsOnFailure(t *testing.T) {
	metrics.ReinitializeAll()
	calls := []testutils.TestCmd{
		{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true, HasStartError: true, ExitCode: 1},
		{Cmd: []string{"grep", "-q", "-v", "azure-npm-"}, ExitCode: 0}, // non-azure sets exist
		{Cmd: []string{"ipset", "list", "--name"}, PipedToCommand: true, HasStartError: true, ExitCode: 1},
		{Cmd: []string{"grep", "azure-npm-"}},
	}
	ioShim := common.NewMockIOShim(calls)
	defer ioShim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioShim)

	iMgr.CreateIPSets([]*IPSetMetadata{namespaceSet, keyLabelOfPodSet})

	metrics.IncNumIPSets()
	metrics.IncNumIPSets()
	metrics.AddEntryToIPSet("test1")
	metrics.AddEntryToIPSet("test1")
	metrics.AddEntryToIPSet("test2")

	require.Error(t, iMgr.ResetIPSets())

	assertExpectedInfo(t, iMgr, &expectedInfo{
		mainCache:        nil,
		toAddUpdateCache: nil,
		toDeleteCache:    nil,
		setsForKernel:    nil,
	})
}

// for applyIPSetsWithSave()
func TestApplyIPSetsSuccessWithoutSave(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, PipedToCommand: true},
		{Cmd: []string{"grep", "azure-npm-"}},
		{Cmd: ipsetRestoreStringSlice},
		{Cmd: ipsetRestoreStringSlice},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	// delete a set so the file isn't empty (otherwise the creator won't even call the exec command)
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata}) // create so we can delete
	err := iMgr.applyIPSetsWithSaveFile()
	require.NoError(t, err)
	iMgr.clearDirtyCache()

	// no sets to add/update, so don't call ipset save
	iMgr.DeleteIPSet(TestNSSet.PrefixName, util.SoftDelete)
	err = iMgr.applyIPSetsWithSaveFile()
	require.NoError(t, err)
}

func TestApplyIPSetsSuccessWithSave(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, PipedToCommand: true},
		{Cmd: []string{"grep", "azure-npm-"}},
		{Cmd: ipsetRestoreStringSlice},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	// create a set so we run ipset save
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	err := iMgr.applyIPSetsWithSaveFile()
	require.NoError(t, err)
}

func TestApplyIPSetsFailureOnSave(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, PipedToCommand: true, HasStartError: true, ExitCode: 1},
		{Cmd: []string{"grep", "azure-npm-"}},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	// create a set so we run ipset save
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	err := iMgr.applyIPSetsWithSaveFile()
	require.Error(t, err)
}

func TestApplyIPSetsFailureOnRestore(t *testing.T) {
	calls := []testutils.TestCmd{
		// fail 5 times because this is our max try count
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)
	// create a set so we run ipset save
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	err := iMgr.applyIPSets()
	require.Error(t, err)

	// same test with save file
	calls = []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, PipedToCommand: true},
		{Cmd: []string{"grep", "azure-npm-"}},
		// fail 5 times because this is our max try count
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
	}
	ioshim = common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr = NewIPSetManager(applyAlwaysCfg, ioshim)
	// create a set so we run ipset save
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	err = iMgr.applyIPSetsWithSaveFile()
	require.Error(t, err)
}

func TestApplyIPSetsRecoveryForFailureOnRestore(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)
	// create a set so we run ipset save
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	err := iMgr.applyIPSets()
	require.NoError(t, err)

	// same test with save file
	calls = []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, PipedToCommand: true},
		{Cmd: []string{"grep", "azure-npm-"}},
		{Cmd: ipsetRestoreStringSlice, ExitCode: 1},
		{Cmd: ipsetRestoreStringSlice},
	}
	ioshim = common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr = NewIPSetManager(applyAlwaysCfg, ioshim)
	// create a set so we run ipset save
	iMgr.CreateIPSets([]*IPSetMetadata{TestNSSet.Metadata})
	err = iMgr.applyIPSetsWithSaveFile()
	require.NoError(t, err)
}

func TestIPSetSave(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, PipedToCommand: true},
		{Cmd: []string{"grep", "azure-npm-"}, Stdout: saveResult},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	output, err := iMgr.ipsetSave()
	require.NoError(t, err)
	require.Equal(t, saveResult, string(output))
}

func TestIPSetSaveNoMatch(t *testing.T) {
	calls := []testutils.TestCmd{
		{Cmd: ipsetSaveStringSlice, ExitCode: 1},
		{Cmd: []string{"grep", "azure-npm-"}},
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	output, err := iMgr.ipsetSave()
	require.NoError(t, err)
	require.Nil(t, output)
}

func TestCreateForAllSetTypes(t *testing.T) {
	tests := []struct {
		name         string
		withSaveFile bool
	}{
		{name: "with save file", withSaveFile: true},
		{name: "no save file", withSaveFile: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
			ioshim := common.NewMockIOShim(calls)
			defer ioshim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
			iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata})
			iMgr.CreateIPSets([]*IPSetMetadata{TestNamedportSet.Metadata})
			iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
			require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
			require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestKVPodSet.Metadata}))
			iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})

			var creator *ioutil.FileCreator
			if tt.withSaveFile {
				creator = iMgr.fileCreatorForApplyWithSaveFile(len(calls), nil)
			} else {
				creator = iMgr.fileCreatorForApply(len(calls))
			}
			actualLines := testAndSortRestoreFileString(t, creator.ToString())

			expectedLines := []string{
				fmt.Sprintf("-N %s --exist nethash", TestNSSet.HashedName),
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				fmt.Sprintf("-N %s --exist nethash", TestKVPodSet.HashedName),
				fmt.Sprintf("-N %s --exist hash:ip,port", TestNamedportSet.HashedName),
				fmt.Sprintf("-N %s --exist nethash maxelem 4294967295", TestCIDRSet.HashedName),
				fmt.Sprintf("-N %s --exist setlist", TestKeyNSList.HashedName),
				fmt.Sprintf("-N %s --exist setlist", TestKVNSList.HashedName),
				fmt.Sprintf("-N %s --exist setlist", TestNestedLabelList.HashedName),
				fmt.Sprintf("-A %s 10.0.0.0", TestNSSet.HashedName),
				fmt.Sprintf("-A %s 10.0.0.1", TestNSSet.HashedName),
				fmt.Sprintf("-A %s 10.0.0.5", TestKeyPodSet.HashedName),
				fmt.Sprintf("-A %s %s", TestKeyNSList.HashedName, TestNSSet.HashedName),
				fmt.Sprintf("-A %s %s", TestKeyNSList.HashedName, TestKeyPodSet.HashedName),
				fmt.Sprintf("-A %s %s", TestKVNSList.HashedName, TestKVPodSet.HashedName),
				"",
			}
			sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

			dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
			wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
			require.NoError(t, err, "ipset restore should be successful")
			require.False(t, wasFileAltered, "file should not be altered")
		})
	}
}

func TestDestroy(t *testing.T) {
	tests := []struct {
		name         string
		withSaveFile bool
	}{
		{name: "with save file", withSaveFile: true},
		{name: "no save file", withSaveFile: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			calls := []testutils.TestCmd{
				fakeRestoreSuccessCommand,
			}
			ioshim := common.NewMockIOShim(calls)
			defer ioshim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

			iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})         // create so we can delete
			iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata}) // create so we can delete
			// clear dirty cache, otherwise a set deletion will be a no-op
			iMgr.clearDirtyCache()

			// remove some members and destroy some sets
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
			require.NoError(t, iMgr.RemoveFromSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
			iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
			require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
			require.NoError(t, iMgr.RemoveFromList(TestKeyNSList.Metadata, []*IPSetMetadata{TestKeyPodSet.Metadata}))
			iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)
			iMgr.DeleteIPSet(TestNestedLabelList.PrefixName, util.SoftDelete)

			var creator *ioutil.FileCreator
			if tt.withSaveFile {
				creator = iMgr.fileCreatorForApplyWithSaveFile(1, nil)
			} else {
				creator = iMgr.fileCreatorForApply(1)
			}
			actualLines := testAndSortRestoreFileString(t, creator.ToString())

			expectedLines := []string{
				fmt.Sprintf("-N %s --exist nethash", TestNSSet.HashedName),
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				fmt.Sprintf("-N %s --exist setlist", TestKeyNSList.HashedName),
				fmt.Sprintf("-A %s 10.0.0.0", TestNSSet.HashedName),
				fmt.Sprintf("-A %s %s", TestKeyNSList.HashedName, TestNSSet.HashedName),
				fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
				fmt.Sprintf("-F %s", TestNestedLabelList.HashedName),
				fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
				fmt.Sprintf("-X %s", TestNestedLabelList.HashedName),
				"",
			}
			sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

			dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
			wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
			require.NoError(t, err, "ipset restore should be successful")
			require.False(t, wasFileAltered, "file should not be altered")
		})
	}
}

// no save file involved
func TestDeleteMembers(t *testing.T) {
	calls := []testutils.TestCmd{
		fakeRestoreSuccessCommand,
	}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "1.1.1.1", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "2.2.2.2", "b"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "3.3.3.3", "c"))
	// create to destroy later
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})
	// clear dirty cache, otherwise a set deletion will be a no-op
	iMgr.clearDirtyCache()

	// will remove this member
	require.NoError(t, iMgr.RemoveFromSets([]*IPSetMetadata{TestNSSet.Metadata}, "1.1.1.1", "a"))
	// will add this member
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "5.5.5.5", "e"))
	// won't add/remove this member since the next two calls cancel each other out
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "4.4.4.4", "d"))
	require.NoError(t, iMgr.RemoveFromSets([]*IPSetMetadata{TestNSSet.Metadata}, "4.4.4.4", "d"))
	// won't add/remove this member since the next two calls cancel each other out
	require.NoError(t, iMgr.RemoveFromSets([]*IPSetMetadata{TestNSSet.Metadata}, "2.2.2.2", "b"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "2.2.2.2", "b"))
	// destroy extra set
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	expectedLines := []string{
		fmt.Sprintf("-N %s --exist nethash", TestNSSet.HashedName),
		fmt.Sprintf("-D %s 1.1.1.1", TestNSSet.HashedName),
		fmt.Sprintf("-A %s 5.5.5.5", TestNSSet.HashedName),
		fmt.Sprintf("-F %s", TestCIDRSet.HashedName),
		fmt.Sprintf("-X %s", TestCIDRSet.HashedName),
		"",
	}
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)
	creator := iMgr.fileCreatorForApply(len(calls))
	actualLines := testAndSortRestoreFileString(t, creator.ToString())
	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err, "ipset restore should be successful")
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestUpdateWithIdenticalSaveFile(t *testing.T) {
	calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createNethashFormat, TestNSSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestNSSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.1", TestNSSet.HashedName),
		fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.5", TestKeyPodSet.HashedName),
		fmt.Sprintf(createPorthashFormat, TestNamedportSet.HashedName),
		fmt.Sprintf(createListFormat, TestKeyNSList.HashedName),
		fmt.Sprintf("add %s %s", TestKeyNSList.HashedName, TestNSSet.HashedName),
		fmt.Sprintf("add %s %s", TestKeyNSList.HashedName, TestKeyPodSet.HashedName),
		fmt.Sprintf(createListFormat, TestKVNSList.HashedName),
		fmt.Sprintf("add %s %s", TestKVNSList.HashedName, TestKVPodSet.HashedName),
		fmt.Sprintf(createListFormat, TestNestedLabelList.HashedName),
	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.5", "c"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestNamedportSet.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestKVPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata})

	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	actualLines := testAndSortRestoreFileString(t, creator.ToString())

	expectedLines := []string{
		fmt.Sprintf("-N %s --exist nethash", TestNSSet.HashedName),
		fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
		fmt.Sprintf("-N %s --exist nethash", TestKVPodSet.HashedName),
		fmt.Sprintf("-N %s --exist hash:ip,port", TestNamedportSet.HashedName),
		fmt.Sprintf("-N %s --exist setlist", TestKeyNSList.HashedName),
		fmt.Sprintf("-N %s --exist setlist", TestKVNSList.HashedName),
		fmt.Sprintf("-N %s --exist setlist", TestNestedLabelList.HashedName),
		"",
	}
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err, "ipset restore should be successful")
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestUpdateWithRealisticSaveFile(t *testing.T) {
	// save file doesn't have some sets we're adding and has some sets that:
	// - aren't dirty
	// - will be deleted
	// - have members which we will delete
	// - are missing members, which we will add
	calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createNethashFormat, TestNSSet.HashedName),                          // should add 10.0.0.1-5 to this set
		fmt.Sprintf("add %s 10.0.0.0", TestNSSet.HashedName),                            // keep this member
		fmt.Sprintf("add %s 5.6.7.8", TestNSSet.HashedName),                             // delete this member
		fmt.Sprintf("add %s 5.6.7.9", TestNSSet.HashedName),                             // delete this member
		fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName),                      // dirty but no member changes in the end
		fmt.Sprintf(createNethashFormat, TestKVPodSet.HashedName),                       // ignore this set since it's not dirty
		fmt.Sprintf("add %s 1.2.3.4", TestKVPodSet.HashedName),                          // ignore this set since it's not dirty
		fmt.Sprintf(createListFormat, TestKeyNSList.HashedName),                         // should add TestKeyPodSet to this set
		fmt.Sprintf("add %s %s", TestKeyNSList.HashedName, TestNSSet.HashedName),        // keep this member
		fmt.Sprintf("add %s %s", TestKeyNSList.HashedName, TestNamedportSet.HashedName), // delete this member
		fmt.Sprintf(createPorthashFormat, TestNamedportSet.HashedName),                  // ignore this set since it's not dirty
		fmt.Sprintf(createListFormat, TestNestedLabelList.HashedName),                   // this set will be deleted
	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	iMgr.CreateIPSets([]*IPSetMetadata{TestNestedLabelList.Metadata}) // create so we can delete
	// clear dirty cache, otherwise a set deletion will be a no-op
	iMgr.clearDirtyCache()

	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "b"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.2", "c"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.3", "d"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.4", "e"))
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.5", "f"))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyPodSet.Metadata})
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata, TestKeyPodSet.Metadata}))
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVNSList.Metadata})
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestCIDRSet.Metadata}, "1.2.3.4", "z")) // set not in save file
	iMgr.DeleteIPSet(TestNestedLabelList.PrefixName, util.SoftDelete)

	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	actualLines := testAndSortRestoreFileString(t, creator.ToString()) // adding NSSet and KeyPodSet (should be keeping NSSet and deleting NamedportSet)

	expectedLines := []string{
		fmt.Sprintf("-N %s --exist nethash", TestNSSet.HashedName),
		fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
		fmt.Sprintf("-N %s --exist setlist", TestKeyNSList.HashedName),
		fmt.Sprintf("-N %s --exist setlist", TestKVNSList.HashedName),
		fmt.Sprintf("-N %s --exist nethash maxelem 4294967295", TestCIDRSet.HashedName),
		fmt.Sprintf("-A %s 1.2.3.4", TestCIDRSet.HashedName),
		fmt.Sprintf("-D %s 5.6.7.8", TestNSSet.HashedName),
		fmt.Sprintf("-D %s 5.6.7.9", TestNSSet.HashedName),
		fmt.Sprintf("-A %s 10.0.0.1", TestNSSet.HashedName),
		fmt.Sprintf("-A %s 10.0.0.2", TestNSSet.HashedName),
		fmt.Sprintf("-A %s 10.0.0.3", TestNSSet.HashedName),
		fmt.Sprintf("-A %s 10.0.0.4", TestNSSet.HashedName),
		fmt.Sprintf("-A %s 10.0.0.5", TestNSSet.HashedName),
		fmt.Sprintf("-D %s %s", TestKeyNSList.HashedName, TestNamedportSet.HashedName),
		fmt.Sprintf("-A %s %s", TestKeyNSList.HashedName, TestKeyPodSet.HashedName),
		fmt.Sprintf("-F %s", TestNestedLabelList.HashedName),
		fmt.Sprintf("-X %s", TestNestedLabelList.HashedName),
		"",
	}
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err, "ipset restore should be successful")
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestHaveTypeProblem(t *testing.T) {
	type args struct {
		metadata *IPSetMetadata
		format   string
	}
	tests := []struct {
		name        string
		args        args
		wantProblem bool
	}{
		{
			name: "correct type for nethash",
			args: args{
				TestNSSet.Metadata,
				createNethashFormat,
			},
			wantProblem: false,
		},
		{
			name: "nethash instead of porthash",
			args: args{
				TestNamedportSet.Metadata,
				createNethashFormat,
			},
			wantProblem: true,
		},
		{
			name: "nethash instead of list",
			args: args{
				TestKeyNSList.Metadata,
				createNethashFormat,
			},
			wantProblem: true,
		},
		{
			name: "correct type for porthash",
			args: args{
				TestNamedportSet.Metadata,
				createPorthashFormat,
			},
			wantProblem: false,
		},
		{
			name: "porthash instead of nethash",
			args: args{
				TestNSSet.Metadata,
				createPorthashFormat,
			},
			wantProblem: true,
		},
		{
			name: "porthash instead of list",
			args: args{
				TestKeyNSList.Metadata,
				createPorthashFormat,
			},
			wantProblem: true,
		},
		{
			name: "correct type for list",
			args: args{
				TestKeyNSList.Metadata,
				createListFormat,
			},
			wantProblem: false,
		},
		{
			name: "list instead of nethash",
			args: args{
				TestNSSet.Metadata,
				createListFormat,
			},
			wantProblem: true,
		},
		{
			name: "list instead of porthash",
			args: args{
				TestNamedportSet.Metadata,
				createListFormat,
			},
			wantProblem: true,
		},
		{
			name: "unknown type",
			args: args{
				TestKeyNSList.Metadata,
				"create %s unknown-type",
			},
			wantProblem: true,
		},
		{
			name: "no rest of line",
			args: args{
				TestKeyNSList.Metadata,
				"create %s",
			},
			wantProblem: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			set := NewIPSet(tt.args.metadata)
			line := fmt.Sprintf(tt.args.format, set.HashedName)
			splitLine := strings.Split(line, " ")
			restOfLine := splitLine[2:]
			if tt.wantProblem {
				require.True(t, haveTypeProblem(set, restOfLine))
			} else {
				require.False(t, haveTypeProblem(set, restOfLine))
			}
		})
	}
}

func TestUpdateWithBadSaveFile(t *testing.T) {
	type args struct {
		dirtySet      []*IPSetMetadata
		saveFileLines []string
	}
	tests := []struct {
		name          string
		args          args
		expectedLines []string
	}{
		{
			name: "no create line",
			args: args{
				[]*IPSetMetadata{TestKeyPodSet.Metadata},
				[]string{
					fmt.Sprintf("add %s 1.1.1.1", TestKeyPodSet.HashedName),
					fmt.Sprintf("add %s 1.1.1.1", TestKeyPodSet.HashedName),
				},
			},
			expectedLines: []string{
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				"",
			},
		},
		{
			name: "unexpected verb after create",
			args: args{
				[]*IPSetMetadata{TestKeyPodSet.Metadata},
				[]string{
					fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName),
					"wrong-verb ...",
				},
			},
			expectedLines: []string{
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				"",
			},
		},
		{
			name: "non-NPM set",
			args: args{
				[]*IPSetMetadata{TestKeyPodSet.Metadata},
				[]string{
					"create test-set1 hash:net family inet hashsize 1024 maxelem 65536",
					"add test-set1 1.2.3.4",
				},
			},
			expectedLines: []string{
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				"",
			},
		},
		{
			name: "ignore set we've already parsed",
			args: args{
				[]*IPSetMetadata{TestKeyPodSet.Metadata},
				[]string{
					fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName), // include
					fmt.Sprintf("add %s 4.4.4.4", TestKeyPodSet.HashedName),    // include this add (will DELETE this member)
					fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName), // ignore this create and ensuing adds since we already included this set
					fmt.Sprintf("add %s 5.5.5.5", TestKeyPodSet.HashedName),    // ignore this add (will NO-OP [no delete])
				},
			},
			expectedLines: []string{
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				fmt.Sprintf("-D %s 4.4.4.4", TestKeyPodSet.HashedName),
				"",
			},
		},
		{
			name: "set with wrong type",
			args: args{
				[]*IPSetMetadata{TestKeyPodSet.Metadata},
				[]string{
					fmt.Sprintf(createPorthashFormat, TestKeyPodSet.HashedName), // ignore since wrong type
					fmt.Sprintf("add %s 1.2.3.4,tcp", TestKeyPodSet.HashedName), // ignore this add (will NO-OP [no delete])
				},
			},
			expectedLines: []string{
				// TODO ideally we shouldn't create this set because the line will fail in the first try for ipset restore
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				"",
			},
		},
		{
			name: "ignore after add with bad parent",
			args: args{
				[]*IPSetMetadata{TestKeyPodSet.Metadata},
				[]string{
					fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName), // include this
					fmt.Sprintf("add %s 7.7.7.7", TestKeyPodSet.HashedName),    // include this add (will DELETE this member)
					fmt.Sprintf("add %s 8.8.8.8", TestNSSet.HashedName),        // ignore this and jump to next create since it's an unexpected set (will NO-OP [no delete])
					fmt.Sprintf("add %s 9.9.9.9", TestKeyPodSet.HashedName),    // ignore add because of error above (will NO-OP [no delete])
				},
			},
			expectedLines: []string{
				fmt.Sprintf("-N %s --exist nethash", TestKeyPodSet.HashedName),
				fmt.Sprintf("-D %s 7.7.7.7", TestKeyPodSet.HashedName),
				"",
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			calls := []testutils.TestCmd{fakeRestoreSuccessCommand}
			ioshim := common.NewMockIOShim(calls)
			defer ioshim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

			saveFileString := strings.Join(tt.args.saveFileLines, "\n")
			saveFileBytes := []byte(saveFileString)

			iMgr.CreateIPSets(tt.args.dirtySet)

			creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
			actualLines := testAndSortRestoreFileString(t, creator.ToString())
			sortedExpectedLines := testAndSortRestoreFileLines(t, tt.expectedLines)

			dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
			wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
			require.NoError(t, err, "ipset restore should be successful")
			require.False(t, wasFileAltered, "file should not be altered")
		})
	}
}

func TestFailureOnCreateForNewSet(t *testing.T) {
	tests := []struct {
		name         string
		withSaveFile bool
	}{
		{name: "with save file", withSaveFile: true},
		{name: "no save file", withSaveFile: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// with respect to the error line, be weary that sets in the save file are processed first and in order, and other sets are processed in random order
			// test logic:
			// - delete a set
			// - create three sets, each with two members. the second set to appear will fail to be created
			errorLineNum := 2
			setToCreateAlreadyExistsCommand := testutils.TestCmd{
				Cmd:      ipsetRestoreStringSlice,
				Stdout:   fmt.Sprintf("Error in line %d: Set cannot be created: set with the same name already exists", errorLineNum),
				ExitCode: 1,
			}
			calls := []testutils.TestCmd{setToCreateAlreadyExistsCommand, fakeRestoreSuccessCommand}
			ioshim := common.NewMockIOShim(calls)
			defer ioshim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

			// add all of these members to the kernel
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKVPodSet.Metadata}, "1.2.3.4", "a"))             // create and add member
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKVPodSet.Metadata}, "1.2.3.5", "b"))             // add member
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestCIDRSet.Metadata}, "1.2.3.4", "a"))              // create and add member
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestCIDRSet.Metadata}, "1.2.3.5", "b"))              // add member
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNamedportSet.Metadata}, "1.2.3.4,tcp:567", "a")) // create and add member
			require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNamedportSet.Metadata}, "1.2.3.5,tcp:567", "b")) // add member
			iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})                                             // create so we can delete
			iMgr.DeleteIPSet(TestKeyNSList.PrefixName, util.SoftDelete)

			// get original creator and run it the first time
			var creator *ioutil.FileCreator
			if tt.withSaveFile {
				creator = iMgr.fileCreatorForApplyWithSaveFile(len(calls), nil)
			} else {
				creator = iMgr.fileCreatorForApply(len(calls))
			}
			originalLines := strings.Split(creator.ToString(), "\n")
			wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
			require.Error(t, err, "ipset restore should fail")
			require.True(t, wasFileAltered, "file should be altered")

			// rerun the creator after removing previously run lines, and aborting the create, adds, and deletes for the second set to updated
			removedSetName := hashedNameOfSetImpacted(t, "-N", originalLines, errorLineNum)
			requireStringInSlice(t, removedSetName, []string{TestNSSet.HashedName, TestKVPodSet.HashedName, TestCIDRSet.HashedName, TestNamedportSet.HashedName})
			expectedLines := originalLines[errorLineNum:] // skip the error line and the lines previously run
			originalLength := len(expectedLines)
			expectedLines = removeOperationsForSet(expectedLines, removedSetName, "-A")
			require.Equal(t, originalLength-2, len(expectedLines), "expected to remove two add lines")
			sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

			actualLines := testAndSortRestoreFileString(t, creator.ToString())
			dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
			wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
			require.NoError(t, err)
			require.False(t, wasFileAltered, "file should not be altered")
		})
	}
}

func TestFailureOnCreateForSetInKernel(t *testing.T) {
	// with respect to the error line, be weary that sets in the save file are processed first and in order, and other sets are processed in random order
	// test logic:
	// - delete a set
	// - update three sets already in the kernel, each with a delete and add line. the second set to appear will fail to be created
	errorLineNum := 2
	setToCreateAlreadyExistsCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   fmt.Sprintf("Error in line %d: Set cannot be created: set with the same name already exists", errorLineNum),
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{setToCreateAlreadyExistsCommand, fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createNethashFormat, TestNSSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestNSSet.HashedName), // delete
		fmt.Sprintf(createNethashFormat, TestKeyPodSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestKeyPodSet.HashedName), // delete
		fmt.Sprintf(createNethashFormat, TestKVPodSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestKVPodSet.HashedName), // delete
	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	// add all of these members to the kernel
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "6.7.8.9", "a"))     // add member to kernel
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "6.7.8.9", "a")) // add member to kernel
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKVPodSet.Metadata}, "6.7.8.9", "a"))  // add member to kernel
	iMgr.CreateIPSets([]*IPSetMetadata{TestKeyNSList.Metadata})                                  // create so we can delete
	iMgr.DeleteIPSet(TestKeyNSList.PrefixName, util.SoftDelete)

	// get original creator and run it the first time
	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	originalLines := strings.Split(creator.ToString(), "\n")
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err, "ipset restore should fail")
	require.True(t, wasFileAltered, "file should be altered")

	// rerun the creator after removing previously run lines, and aborting the create, adds, and deletes for the second set to updated
	removedSetName := hashedNameOfSetImpacted(t, "-N", originalLines, errorLineNum)
	requireStringInSlice(t, removedSetName, []string{TestNSSet.HashedName, TestKeyPodSet.HashedName, TestKVPodSet.HashedName})
	expectedLines := originalLines[errorLineNum:] // skip the error line and the lines previously run
	originalLength := len(expectedLines)
	expectedLines = removeOperationsForSet(expectedLines, removedSetName, "-D")
	require.Equal(t, originalLength-1, len(expectedLines), "expected to remove a delete line")
	expectedLines = removeOperationsForSet(expectedLines, removedSetName, "-A")
	require.Equal(t, originalLength-2, len(expectedLines), "expected to remove an add line")
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	actualLines := testAndSortRestoreFileString(t, creator.ToString())
	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestFailureOnAddToListInKernel(t *testing.T) {
	// with respect to the error line, be weary that sets in the save file are processed first and in order, and other sets are processed in random order
	// test logic:
	// - delete a set
	// - update three lists already in the set, each with a delete and add line. the second list to appear will have the failed add
	// - create a set and add a member to it
	errorLineNum := 8
	memberDoesNotExistCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   fmt.Sprintf("Error in line %d: Set to be added/deleted/tested as element does not exist", errorLineNum), // this error might happen if the cache is out of date with the kernel
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{memberDoesNotExistCommand, fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createListFormat, TestKeyNSList.HashedName),
		fmt.Sprintf("add %s %s", TestKeyNSList.HashedName, TestNSSet.HashedName), // delete this member
		fmt.Sprintf(createListFormat, TestKVNSList.HashedName),
		fmt.Sprintf("add %s %s", TestKVNSList.HashedName, TestNSSet.HashedName), // delete this member
		fmt.Sprintf(createListFormat, TestNestedLabelList.HashedName),
		fmt.Sprintf("add %s %s", TestNestedLabelList.HashedName, TestNSSet.HashedName), // delete this member

	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.0", "a"))                                 // create and add member
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestKeyPodSet.Metadata}))       // add member to kernel
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestKeyPodSet.Metadata}))        // add member to kernel
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestNestedLabelList.Metadata}, []*IPSetMetadata{TestKeyPodSet.Metadata})) // add member to kernel
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})                                                                     // create so we can delete
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	originalLines := strings.Split(creator.ToString(), "\n")
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err, "ipset restore should fail")
	require.True(t, wasFileAltered, "file should be altered")

	// rerun the creator after removing previously run lines, and aborting the member-add line that failed
	removedSetName := hashedNameOfSetImpacted(t, "-A", originalLines, errorLineNum)
	requireStringInSlice(t, removedSetName, []string{TestKeyNSList.HashedName, TestKVNSList.HashedName, TestNestedLabelList.HashedName})
	removedMember := memberNameOfSetImpacted(t, originalLines, errorLineNum)
	require.Equal(t, TestKeyPodSet.HashedName, removedMember)
	expectedLines := originalLines[errorLineNum:] // skip the error line and the lines previously run
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	actualLines := testAndSortRestoreFileString(t, creator.ToString())
	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestFailureOnAddToNewList(t *testing.T) {
	// with respect to the error line, be weary that sets in the save file are processed first and in order, and other sets are processed in random order
	// test logic:
	// - delete a set
	// - update a set already in the kernel with a delete and add line
	// - create three lists in the set, each with an add line. the second list to appear will have the failed add
	errorLineNum := 8
	memberDoesNotExistCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   fmt.Sprintf("Error in line %d: Set to be added/deleted/tested as element does not exist", errorLineNum), // this error might happen if the cache is out of date with the kernel
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{memberDoesNotExistCommand, fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createNethashFormat, TestNSSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestNSSet.HashedName), // delete this member
	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.1", "a"))                                 // create and add member
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKeyNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata}))       // add member to kernel
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestKVNSList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata}))        // add member to kernel
	require.NoError(t, iMgr.AddToLists([]*IPSetMetadata{TestNestedLabelList.Metadata}, []*IPSetMetadata{TestNSSet.Metadata})) // add member to kernel
	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})                                                                 // create so we can delete
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	originalLines := strings.Split(creator.ToString(), "\n")
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err, "ipset restore should fail")
	require.True(t, wasFileAltered, "file should be altered")

	// rerun the creator after removing previously run lines, and aborting the member-add line that failed
	removedSetName := hashedNameOfSetImpacted(t, "-A", originalLines, errorLineNum)
	requireStringInSlice(t, removedSetName, []string{TestKeyNSList.HashedName, TestKVNSList.HashedName, TestNestedLabelList.HashedName})
	removedMember := memberNameOfSetImpacted(t, originalLines, errorLineNum)
	require.Equal(t, TestNSSet.HashedName, removedMember)
	expectedLines := originalLines[errorLineNum:] // skip the error line and the lines previously run
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	actualLines := testAndSortRestoreFileString(t, creator.ToString())
	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestFailureOnDelete(t *testing.T) {
	// TODO
}

func TestFailureOnFlush(t *testing.T) {
	// test logic:
	// - delete two sets. the first to appear will fail to flush
	// - update a set by deleting a member
	// - create a set with a member
	errorLineNum := 5
	setDoesNotExistCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   fmt.Sprintf("Error in line %d: The set with the given name does not exist", errorLineNum), // this error might happen if the cache is out of date with the kernel
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{setDoesNotExistCommand, fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createNethashFormat, TestNSSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestNSSet.HashedName), // keep this member
		fmt.Sprintf("add %s 10.0.0.1", TestNSSet.HashedName), // delete this member
	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})  // create so we can delete
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata}) // create so we can delete
	// clear dirty cache, otherwise a set deletion will be a no-op
	iMgr.clearDirtyCache()

	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))     // in kernel already
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.0", "a")) // not in kernel yet
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName, util.SoftDelete)
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	originalLines := strings.Split(creator.ToString(), "\n")
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err, "ipset restore should fail")
	require.True(t, wasFileAltered, "file should be altered")

	// rerun the creator after aborting the flush and delete for the set that failed to flush
	removedSetName := hashedNameOfSetImpacted(t, "-F", originalLines, errorLineNum)
	requireStringInSlice(t, removedSetName, []string{TestKVPodSet.HashedName, TestCIDRSet.HashedName})
	expectedLines := originalLines[errorLineNum:] // skip the error line and the lines previously run
	originalLength := len(expectedLines)
	expectedLines = removeOperationsForSet(expectedLines, removedSetName, "-X")
	require.Equal(t, originalLength-1, len(expectedLines), "expected to remove one destroy line")
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	actualLines := testAndSortRestoreFileString(t, creator.ToString())
	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestFailureOnDestroy(t *testing.T) {
	// test logic:
	// - delete two sets. the first to appear will fail to delete
	// - update a set by deleting a member
	// - create a set with a member
	errorLineNum := 7
	inUseByKernelCommand := testutils.TestCmd{
		Cmd:      ipsetRestoreStringSlice,
		Stdout:   fmt.Sprintf("Error in line %d: Set cannot be destroyed: it is in use by a kernel component", errorLineNum),
		ExitCode: 1,
	}
	calls := []testutils.TestCmd{inUseByKernelCommand, fakeRestoreSuccessCommand}
	ioshim := common.NewMockIOShim(calls)
	defer ioshim.VerifyCalls(t, calls)
	iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

	saveFileLines := []string{
		fmt.Sprintf(createNethashFormat, TestNSSet.HashedName),
		fmt.Sprintf("add %s 10.0.0.0", TestNSSet.HashedName), // keep this member
		fmt.Sprintf("add %s 10.0.0.1", TestNSSet.HashedName), // delete this member
	}
	saveFileString := strings.Join(saveFileLines, "\n")
	saveFileBytes := []byte(saveFileString)

	iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata})  // create so we can delete
	iMgr.CreateIPSets([]*IPSetMetadata{TestKVPodSet.Metadata}) // create so we can delete
	// clear dirty cache, otherwise a set deletion will be a no-op
	iMgr.clearDirtyCache()

	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestNSSet.Metadata}, "10.0.0.0", "a"))     // in kernel already
	require.NoError(t, iMgr.AddToSets([]*IPSetMetadata{TestKeyPodSet.Metadata}, "10.0.0.0", "a")) // not in kernel yet
	iMgr.DeleteIPSet(TestKVPodSet.PrefixName, util.SoftDelete)
	iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

	creator := iMgr.fileCreatorForApplyWithSaveFile(len(calls), saveFileBytes)
	originalLines := strings.Split(creator.ToString(), "\n")
	wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
	require.Error(t, err, "ipset restore should fail")
	require.True(t, wasFileAltered, "file should be altered")

	removedSetName := hashedNameOfSetImpacted(t, "-X", originalLines, errorLineNum)
	requireStringInSlice(t, removedSetName, []string{TestKVPodSet.HashedName, TestCIDRSet.HashedName})
	expectedLines := originalLines[errorLineNum:] // skip the error line and the lines previously run
	sortedExpectedLines := testAndSortRestoreFileLines(t, expectedLines)

	actualLines := testAndSortRestoreFileString(t, creator.ToString())
	dptestutils.AssertEqualLines(t, sortedExpectedLines, actualLines)
	wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
	require.NoError(t, err)
	require.False(t, wasFileAltered, "file should not be altered")
}

func TestFailureOnLastLine(t *testing.T) {
	tests := []struct {
		name         string
		withSaveFile bool
	}{
		{name: "with save file", withSaveFile: true},
		{name: "no save file", withSaveFile: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// make sure that the file recovers and returns no error when there are no more lines on the second run
			// test logic:
			// - delete a set
			errorLineNum := 2
			calls := []testutils.TestCmd{
				{
					Cmd:      ipsetRestoreStringSlice,
					Stdout:   fmt.Sprintf("Error in line %d: some destroy error", errorLineNum),
					ExitCode: 1,
				},
			}
			ioshim := common.NewMockIOShim(calls)
			defer ioshim.VerifyCalls(t, calls)
			iMgr := NewIPSetManager(applyAlwaysCfg, ioshim)

			iMgr.CreateIPSets([]*IPSetMetadata{TestCIDRSet.Metadata}) // create so we can delete
			// clear dirty cache, otherwise a set deletion will be a no-op
			iMgr.clearDirtyCache()

			iMgr.DeleteIPSet(TestCIDRSet.PrefixName, util.SoftDelete)

			var creator *ioutil.FileCreator
			if tt.withSaveFile {
				creator = iMgr.fileCreatorForApplyWithSaveFile(2, nil)
			} else {
				creator = iMgr.fileCreatorForApply(2)
			}
			wasFileAltered, err := creator.RunCommandOnceWithFile("ipset", "restore")
			require.Error(t, err, "ipset restore should fail")
			require.True(t, wasFileAltered, "file should be altered")

			expectedLines := []string{""} // skip the error line and the lines previously run
			actualLines := testAndSortRestoreFileString(t, creator.ToString())
			dptestutils.AssertEqualLines(t, expectedLines, actualLines)
			wasFileAltered, err = creator.RunCommandOnceWithFile("ipset", "restore")
			require.NoError(t, err)
			require.False(t, wasFileAltered, "file should not be altered")
		})
	}
}

func testAndSortRestoreFileString(t *testing.T, multilineString string) []string {
	return testAndSortRestoreFileLines(t, strings.Split(multilineString, "\n"))
}

// make sure file goes in order of creates, adds/deletes, flushes, then destroys
// then sort those sections and return the lines in an array
func testAndSortRestoreFileLines(t *testing.T, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	require.True(t, lines[len(lines)-1] == "", "restore file must end with blank line")
	lines = lines[:len(lines)-1] // remove the blank line

	// order of operation groups in restore file (can have groups with multiple possible operatoins)
	operationGroups := [][]string{
		{"-N"},       // creates
		{"-A", "-D"}, // adds/deletes
		{"-F"},       // flushes
		{"-X"},       // destroys
	}
	result := make([]string, 0, len(lines))
	groupIndex := 0
	groupStartIndex := 0
	k := 0
	for k < len(lines) {
		for k < len(lines) {
			// iterate until we reach an operation not in the current operation group
			operation := lines[k][0:2]
			expectedOperations := operationGroups[groupIndex]
			if !isStringInSlice(operation, expectedOperations) {
				require.True(t, groupIndex < len(operationGroups)-1, "ran out of operation groups. got operation %s", operation)
				operationLines := lines[groupStartIndex:k]
				sort.Strings(operationLines)
				result = append(result, operationLines...)
				groupStartIndex = k
				groupIndex++
				break
			}
			k++
		}
	}
	// add the remaining lines since the final operation group won't pass through the if statement in the loop above
	operatrionLines := lines[groupStartIndex:]
	sort.Strings(operatrionLines)
	result = append(result, operatrionLines...)
	result = append(result, "") // add the blank line
	return result
}

func hashedNameOfSetImpacted(t *testing.T, operation string, lines []string, lineNum int) string {
	lineNumIndex := lineNum - 1
	line := lines[lineNumIndex]
	pattern := fmt.Sprintf(`\%s (azure-npm-\d+)`, operation)
	re := regexp.MustCompile(pattern)
	results := re.FindStringSubmatch(line)
	require.Equal(t, 2, len(results), "expected to find a match with regex pattern %s for line: %s", pattern, line)
	return results[1] // second item in slice is the group surrounded by ()
}

func memberNameOfSetImpacted(t *testing.T, lines []string, lineNum int) string {
	lineNumIndex := lineNum - 1
	line := lines[lineNumIndex]
	pattern := `\-[AD] azure-npm-\d+ (.*)`
	re := regexp.MustCompile(pattern)
	member := re.FindStringSubmatch(line)[1]
	results := re.FindStringSubmatch(line)
	require.Equal(t, 2, len(results), "expected to find a match with regex pattern %s for line: %s", pattern, line)
	return member
}

func isStringInSlice(item string, values []string) bool {
	success := false
	for _, value := range values {
		if item == value {
			success = true
			break
		}
	}
	return success
}

func requireStringInSlice(t *testing.T, item string, values []string) {
	require.Truef(t, isStringInSlice(item, values), "item %s was not one of the possible values %+v", item, values)
}

// remove lines that start with the operation (include the dash in the operations) e.g.
// -A <setname> 1.2.3.4
// -D <setname> 1.2.3.4
// -X <setname>
func removeOperationsForSet(lines []string, hashedSetName, operation string) []string {
	operationRegex := regexp.MustCompile(fmt.Sprintf(`\%s %s`, operation, hashedSetName))
	goodLines := []string{}
	for _, line := range lines {
		if !operationRegex.MatchString(line) {
			goodLines = append(goodLines, line)
		}
	}
	return goodLines
}
