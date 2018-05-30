// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"fmt"
	"io"
	"log"
	"log/syslog"
	"os"
)

// SetTarget sets the log target.
func (logger *Logger) SetTarget(target int) error {
	var err error

	switch target {
	case TargetStdout:
		logger.out = os.Stdout
		break
	case TargetStderr:
		logger.out = os.Stderr
		break
	case TargetSyslog:
		logger.out, err = syslog.New(log.LstdFlags, logger.name)
		break
	case TargetLogfile:
		logger.out, err = os.OpenFile(logger.getLogFileName(), os.O_CREATE|os.O_APPEND|os.O_RDWR, logFilePerm)
		break
	case TargetMultiWrite:
		logger.out, err = os.OpenFile(logger.getLogFileName(), os.O_CREATE|os.O_APPEND|os.O_RDWR, logFilePerm)
		mw := io.MultiWriter(os.Stdout, logger.out)
		if err == nil {
			logger.l.SetOutput(mw)
			logger.target = target
			return nil
		}

		break
	default:
		err = fmt.Errorf("Invalid log target %d", target)
	}

	if err == nil {
		logger.l.SetOutput(logger.out)
		logger.target = target
	}

	return err
}
