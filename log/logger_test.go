// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"os"
	"testing"
)

const (
	logName = "test"
)

// Tests that the log file rotates when size limit is reached.
func TestLogFileRotatesWhenSizeLimitIsReached(t *testing.T) {
	l := NewLogger(logName, LevelInfo, TargetLogfile)
	if l == nil {
		t.Fatalf("Failed to create logger.\n")
	}

	l.SetLogFileLimits(512, 2)

	for i := 1; i <= 100; i++ {
		l.Printf("LogText %v", i)
	}

	l.Close()

	fn := logName + ".log"
	_, err := os.Stat(fn)
	if err != nil {
		t.Errorf("Failed to find active log file.")
	}
	os.Remove(fn)

	fn = logName + ".log.1"
	_, err = os.Stat(fn)
	if err != nil {
		t.Errorf("Failed to find the 1st rotated log file.")
	}
	os.Remove(fn)

	fn = logName + ".log.2"
	_, err = os.Stat(fn)
	if err == nil {
		t.Errorf("Found the 2nd rotated log file which should have been deleted.")
	}
	os.Remove(fn)
}
