// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"fmt"
	"log"
	"log/syslog"
	"os"
)

const (
	// LogPath is the path where log files are stored.
	LogPath = "/var/log/"
)

// SetTarget sets the log target.
func (logger *Logger) SetTarget(target int) error {
	var err error

	switch target {
	case TargetStderr:
		logger.out = os.Stderr
	case TargetSyslog:
		logger.out, err = syslog.New(log.LstdFlags, logger.name)
	case TargetLogfile:
		logger.out, err = os.OpenFile(logger.getLogFileName(), os.O_CREATE|os.O_APPEND|os.O_RDWR, logFilePerm)
	default:
		err = fmt.Errorf("Invalid log target %d", target)
	}

	if err == nil {
		logger.l.SetOutput(logger.out)
		logger.target = target
	}

	return err
}
