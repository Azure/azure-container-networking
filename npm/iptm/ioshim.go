package iptm

import (
	"io"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/util"
	"k8s.io/apimachinery/pkg/util/wait"
)

type ioshim interface {
	GrabIptablesLocks() (*os.File, error)
	SaveConfigFile(configFile string) (io.Writer, error)
	OpenConfigFile(configFile string) (io.Reader, error)
	CloseConfigFile() error
}

type IptOperationShim struct {
	f *os.File
}

func NewIptOperationShim() *IptOperationShim {
	return &IptOperationShim{}
}

func (i *IptOperationShim) SaveConfigFile(configFile string) (io.Writer, error) {
	f, err := os.Create(configFile)
	if err != nil {
		return f, err
	}
	i.f = f
	return i.f, err
}

func (i *IptOperationShim) OpenConfigFile(configFile string) (io.Reader, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return f, err
	}
	i.f = f
	return i.f, err
}

func (i *IptOperationShim) CloseConfigFile() error {
	return i.f.Close()
}

// grabs iptables v1.6 xtable lock
func (i *IptOperationShim) GrabIptablesLocks() (*os.File, error) {
	var success bool

	l := &os.File{}
	defer func(l *os.File) {
		// Clean up immediately on failure
		if !success {
			l.Close()
		}
	}(l)

	// Grab 1.6.x style lock.
	l, err := os.OpenFile(util.IptablesLockFile, os.O_CREATE, 0600)
	if err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to open iptables lock file %s.", util.IptablesLockFile)
		return nil, err
	}

	if err := wait.PollImmediate(200*time.Millisecond, 2*time.Second, func() (bool, error) {
		if err := grabIptablesFileLock(l); err != nil {
			return false, nil
		}

		return true, nil
	}); err != nil {
		metrics.SendErrorLogAndMetric(util.IptmID, "Error: failed to acquire new iptables lock: %v.", err)
		return nil, err
	}

	success = true
	return l, nil
}
