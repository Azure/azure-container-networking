// technically should have a build constraint for !windows, but iptm.go imports this, and there is no build constraint for iptm.go

package ioutil

import (
	"fmt"

	utilexec "k8s.io/utils/exec"
)

// Grep is the grep command string
const (
	Grep                 = "grep"
	GrepRegexFlag        = "-P"
	GrepOnlyMatchingFlag = "-o"
	GrepAntiMatchFlag    = "-v"
	GrepQuietFlag        = "-q"
	GrepBeforeFlag       = "-B"
)

func DoublePipeToGrep(command, grepCommand1, grepCommand2 utilexec.Cmd) (searchResults []byte, gotMatches bool, commandError error) {
	pipe, commandError := command.StdoutPipe()
	if commandError != nil {
		return
	}
	grepCommand1.SetStdin(pipe)

	grepPipe, commandError := grepCommand1.StdoutPipe()
	if commandError != nil {
		_ = pipe.Close()
		commandError = fmt.Errorf("error getting pipe for grep command: %w", commandError)
		return
	}
	grepCommand2.SetStdin(grepPipe)

	closePipes := func() {
		_ = grepPipe.Close()
		_ = pipe.Close()
	}
	defer closePipes()

	commandError = grepCommand1.Start()
	if commandError != nil {
		commandError = fmt.Errorf("error while starting first grep command: %w", commandError)
		return
	}

	commandError = command.Start()
	if commandError != nil {
		commandError = fmt.Errorf("error while starting command: %w", commandError)
		_ = grepCommand1.Wait()
		return
	}

	waitForAll := func() {
		_ = command.Wait()
		_ = grepCommand1.Wait()
	}
	defer waitForAll()

	output, err := grepCommand2.CombinedOutput()
	if err != nil {
		// grep returns err status 1 if nothing is found
		return
	}
	searchResults = output
	gotMatches = true
	return searchResults, gotMatches, commandError // include named results to appease lint
}

func PipeCommandToGrep(command, grepCommand utilexec.Cmd) (searchResults []byte, gotMatches bool, commandError error) {
	pipe, commandError := command.StdoutPipe()
	if commandError != nil {
		return
	}
	closePipe := func() { _ = pipe.Close() } // appease go lint
	defer closePipe()

	grepCommand.SetStdin(pipe)
	commandError = command.Start()
	if commandError != nil {
		return
	}

	// Without this wait, defunct iptable child process are created
	wait := func() { _ = command.Wait() } // appease go lint
	defer wait()

	output, err := grepCommand.CombinedOutput()
	if err != nil {
		// grep returns err status 1 if nothing is found
		// but the other command's exit status gets propagated through this CombinedOutput, so we might have errors undetected
		return
	}
	searchResults = output
	gotMatches = true
	return
}
