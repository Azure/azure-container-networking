// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

// Standard logger is a pre-defined logger for convenience.
// Set log directory as the current location
var stdLog = NewLogger("azure-container-networking", LevelInfo, TargetStderr, "")

// GetStd - Helper functions for the standard logger.
func GetStd() *Logger {
	return stdLog
}

func SetName(name string) {
	stdLog.SetName(name)
}

func SetLevel(level int) {
	stdLog.SetLevel(level)
}

func SetLogFileLimits(maxFileSize int, maxFileCount int) {
	stdLog.SetLogFileLimits(maxFileSize, maxFileCount)
}

func Close() {
	stdLog.Close()
}

func SetTargetLogDirectory(target int, logDirectory string) error {
	return stdLog.SetTargetLogDirectory(target, logDirectory)
}

func GetLogDirectory() string {
	return stdLog.GetLogDirectory()
}

func Request(tag string, request interface{}, err error) {
	stdLog.Request(tag, request, err)
}

func Response(tag string, response interface{}, returnCode int, returnStr string, err error) {
	stdLog.Response(tag, response, returnCode, returnStr, nil, err)
}

// Logf logs to the local log.
func Logf(format string, args ...interface{}) {
	stdLog.Logf(format, args...)
}

// Printf logs to the local log and send the log through the channel.
func Printf(format string, args ...interface{}) {
	stdLog.Printf(format, args...)
}

func Debugf(format string, args ...interface{}) {
	stdLog.Debugf(format, args...)
}

func Errorf(format string, args ...interface{}) {
	stdLog.Errorf(format, args...)
}
