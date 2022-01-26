// technically should have a build constraint for !windows, but iptm.go imports this, and there is no build constraint for iptm.go

package ioutil

import utilexec "k8s.io/utils/exec"

// Grep is the grep command string
const Grep = "grep"

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
