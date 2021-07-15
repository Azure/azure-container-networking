// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"sync"
)

// Log level
const (
	LevelAlert = iota
	LevelError
	LevelWarning
	LevelInfo
	LevelDebug
)

// Log target
const (
	TargetStderr = iota
	TargetSyslog
	TargetLogfile
	TargetStdout
	TargetStdOutAndLogFile
)

const (
	// Log file properties.
	logPrefix        = ""
	logFileExtension = ".log"
	logFilePerm      = os.FileMode(0664)

	// Log file rotation default limits, in bytes.
	maxLogFileSize   = 5 * 1024 * 1024
	maxLogFileCount  = 8
	rotationCheckFrq = 8
)

// Logger object
type Logger struct {
	l            *log.Logger
	out          io.WriteCloser
	name         string
	level        int
	target       int
	maxFileSize  int
	maxFileCount int
	callCount    int
	directory    string
	mutex        *sync.Mutex
}

var pid = os.Getpid()

// NewLogger creates a new Logger.
func NewLogger(name string, level int, target int, logDir string) *Logger {
	var logger Logger

	logger.l = log.New(nil, logPrefix, log.LstdFlags)
	logger.name = name
	logger.level = level
	logger.directory = logDir
	logger.SetTarget(target)
	logger.maxFileSize = maxLogFileSize
	logger.maxFileCount = maxLogFileCount
	logger.mutex = &sync.Mutex{}

	return &logger
}

// SetName sets the log name.
func (logger *Logger) SetName(name string) {
	logger.name = name
}

// SetLevel sets the log chattiness.
func (logger *Logger) SetLevel(level int) {
	logger.level = level
}

// SetLogFileLimits sets the log file limits.
func (logger *Logger) SetLogFileLimits(maxFileSize int, maxFileCount int) {
	logger.maxFileSize = maxFileSize
	logger.maxFileCount = maxFileCount
}

// Close closes the log stream.
func (logger *Logger) Close() {
	if logger.out != nil {
		logger.out.Close()
	}
}

// SetTargetLogDirectory sets the directory location where logs should be stored along with the target
func (logger *Logger) SetTargetLogDirectory(target int, logDirectory string) error {
	logger.directory = logDirectory
	return logger.SetTarget(target)
}

// GetLogDirectory gets the directory location where logs should be stored.
func (logger *Logger) GetLogDirectory() string {
	if logger.directory != "" {
		return logger.directory
	}

	return LogPath
}

// GetLogFileName returns the full log file name.
func (logger *Logger) getLogFileName() string {
	var logFileName string

	if logger.directory != "" {
		logFileName = path.Join(logger.directory, logger.name+logFileExtension)
	} else {
		logFileName = LogPath + logger.name + logFileExtension
	}

	return logFileName
}

// todo: handle errors and use atomic file rotation
// Rotate checks the active log file size and rotates log files if necessary.
func (logger *Logger) rotate() {
	// Return if target is not a log file.
	if (logger.target != TargetLogfile && logger.target != TargetStdOutAndLogFile) || logger.out == nil {
		return
	}

	fileName := logger.getLogFileName()
	fileInfo, err := os.Stat(fileName)
	if err != nil {
		logger.Logf("[log] Failed to query log file info %+v.", err)
		return
	}

	// Rotate if size limit is reached.
	if fileInfo.Size() >= int64(logger.maxFileSize) {
		logger.out.Close()
		var fn1, fn2 string

		// Rotate log files, keeping the last maxFileCount files.
		for n := logger.maxFileCount - 1; n >= 0; n-- {
			fn2 = fn1
			if n == 0 {
				fn1 = fileName
			} else {
				fn1 = fmt.Sprintf("%v.%v", fileName, n)
			}
			if fn2 != "" {
				os.Rename(fn1, fn2)
			}
		}

		// Create a new log file.
		logger.SetTarget(logger.target)
	}
}

// Request logs a structured request.
func (logger *Logger) Request(tag string, request interface{}, err error) {
	if err == nil {
		logger.Printf("[%s] Received %T %+v.", tag, request, request)
	} else {
		logger.Errorf("[%s] Failed to decode %T %+v %s.", tag, request, request, err.Error())
	}
}

// Response logs a structured response.
func (logger *Logger) Response(tag string, response interface{}, returnCode int, returnStr string, err error) {
	if err == nil && returnCode == 0 {
		logger.Printf("[%s] Sent %T %+v.", tag, response, response)
	} else if err != nil {
		logger.Errorf("[%s] Code:%s, %+v %s.", tag, returnStr, response, err.Error())
	} else {
		logger.Errorf("[%s] Code:%s, %+v.", tag, returnStr, response)
	}
}

// ResponseEx logs a structured response and the request associate with it.
func (logger *Logger) ResponseEx(tag string, request interface{}, response interface{}, returnCode int, returnStr string, err error) {
	if err == nil && returnCode == 0 {
		logger.Printf("[%s] Sent %T %+v %T %+v.", tag, request, request, response, response)
	} else if err != nil {
		logger.Errorf("[%s] Code:%s, %+v, %+v %s.", tag, returnStr, request, response, err.Error())
	} else {
		logger.Errorf("[%s] Code:%s, %+v, %+v.", tag, returnStr, request, response)
	}
}

// logf logs a formatted string.
func (logger *Logger) logf(format string, args ...interface{}) {
	if logger.callCount%rotationCheckFrq == 0 {
		logger.rotate()
	}
	format = fmt.Sprintf("[%v] %s", pid, format)
	logger.callCount++
	logger.l.Printf(format, args...)
}

// Logf wraps logf.
func (logger *Logger) Logf(format string, args ...interface{}) {
	logger.mutex.Lock()
	logger.logf(format, args...)
	logger.mutex.Unlock()
}

// Printf logs a formatted string at info level.
func (logger *Logger) Printf(format string, args ...interface{}) {
	if logger.level < LevelInfo {
		return
	}

	logger.Logf(format, args...)
}

// Debugf logs a formatted string at info level.
func (logger *Logger) Debugf(format string, args ...interface{}) {
	if logger.level < LevelDebug {
		return
	}

	logger.Logf(format, args...)
}

// Errorf logs a formatted string at info level and sends the string to TelemetryBuffer.
func (logger *Logger) Errorf(format string, args ...interface{}) {
	logger.Logf(format, args...)
}
