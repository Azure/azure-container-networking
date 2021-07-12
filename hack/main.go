package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	converter "github.com/Azure/azure-container-networking/hack/dataplaneConverter"
	"github.com/Azure/azure-container-networking/log"
	"github.com/Azure/azure-container-networking/npm/iptm"
	"github.com/Azure/azure-container-networking/npm/util"
)

func SaveIntoBuffer(tableName string, buffer *bytes.Buffer) error {
	l, err := iptm.GrabIptablesLocks()
	if err != nil {
		return err
	}

	defer func(l *os.File) {
		if err = l.Close(); err != nil {
			log.Logf("Failed to close iptables locks")
		}
	}(l)

	cmdArgs := []string{"-t", string(tableName)}
	cmd := exec.Command(util.IptablesSave, cmdArgs...)

	cmd.Stdout = buffer
	stderrBuffer := bytes.NewBuffer(nil)
	cmd.Stderr = stderrBuffer

	err = cmd.Run()

	if err != nil {
		stderrBuffer.WriteTo(buffer) // ignore error, since we need to return the original error
	}
	return err
}

func main() {
	var (
		// If required, we can move this buffer one stage up
		// and built as required overtime
		iptableBuffer = bytes.NewBuffer(nil)
		tableName     = "filter"
	)
	if err := SaveIntoBuffer(tableName, iptableBuffer); err != nil {
		// metrics.SendErrorLogAndMetric(util.IptmID, "[BulkUpdateIPtables] Error: failed to get iptables-save command output with err: %s", err.Error())
		fmt.Println(err.Error())
	}
	// p := &parser.Parser{}
	// iptableObj := p.ParseIptablesObject(tableName, iptableBuffer)
	// iptableObj.PrintIptable()

	c := &converter.Converter{}
	ipTableRulesProtoBuffList := c.GetRulesFromIptable(tableName, iptableBuffer)
	fmt.Printf("%s\n", ipTableRulesProtoBuffList)
	// fmt.Printf("%s\n", c.ConvertIptablesObject(iptableObj))
}
