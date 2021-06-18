package iptm

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

type ioshim interface {
	lockIptables() error
	unlockIptables() error
	openConfigFile(configFile string) (io.Reader, error)
	createConfigFile(configFile string) (io.Writer, error)
	closeConfigFile() error
}

type IptOperationShim struct {
	configFile   *os.File
	iptablesLock *os.File
}

func NewIptOperationShim() *IptOperationShim {
	return &IptOperationShim{}
}

func (i *IptOperationShim) createConfigFile(configFile string) (io.Writer, error) {
	f, err := os.Create(configFile)
	if err != nil {
		return f, err
	}
	i.configFile = f
	return i.configFile, err
}

func (i *IptOperationShim) openConfigFile(configFile string) (io.Reader, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return f, err
	}
	i.configFile = f
	return i.configFile, err
}

func (i *IptOperationShim) closeConfigFile() error {
	return i.configFile.Close()
}

// grabs iptables v1.6 xtable lock
func (i *IptOperationShim) lockIptables() error {
	var success bool

	i.iptablesLock = &os.File{}
	defer func(l *os.File) {
		// Clean up immediately on failure
		if !success {
			l.Close()
		}
	}(i.iptablesLock)

	// Grab 1.6.x style lock.
	var err error
	i.iptablesLock, err = os.OpenFile(util.IptablesLockFile, os.O_CREATE, 0600)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to open iptables lock file %s.", util.IptablesLockFile)
		return err
	}

	if err := wait.PollImmediate(200*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := grabIptablesFileLock(i.iptablesLock); err != nil {
			return false, nil
		}

		return true, nil
	}); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to acquire new iptables lock: %v.", err)
		return err
	}

	success = true
	return nil
}

func (i *IptOperationShim) unlockIptables() error {
	if err := i.iptablesLock.Close(); err != nil {
		return fmt.Errorf("Failed to close iptables locks")
	}
	return nil
}
