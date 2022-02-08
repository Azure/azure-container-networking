// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
)

const (
	logName = "test"
)

func TestNewLoggerError(t *testing.T) {
	// we expect an error from NewLoggerE in the event that we provide an
	// unwriteable directory

	// this test needs a guaranteed empty directory, so we create a temporary one
	// and ensure that it gets destroyed afterward.
	targetDir, err := os.MkdirTemp("", "acn")
	if err != nil {
		t.Fatal("unable to create temporary directory: err:", err)
	}

	t.Cleanup(func() {
		// This removal could produce an error, but since it's a temporary
		// directory anyway, this is a best-effort cleanup
		os.Remove(targetDir)
	})

	// if we just use the targetDir, NewLoggerE will create the file and it will
	// work. We need a non-existent directory *within* the tempdir
	fullPath := path.Join(targetDir, "definitelyDoesNotExist")

	_, err = NewLoggerE(logName, LevelInfo, TargetLogfile, fullPath)
	if err == nil {
		t.Error("expected an error but did not receive one")
	}
}

// Tests that the log file rotates when size limit is reached.
func TestLogFileRotatesWhenSizeLimitIsReached(t *testing.T) {
	logDirectory := "" // This sets the current location for logs
	l := NewLogger(logName, LevelInfo, TargetLogfile, logDirectory)
	if l == nil {
		t.Fatalf("Failed to create logger.\n")
	}

	l.SetLogFileLimits(512, 2)

	for i := 1; i <= 100; i++ {
		l.Logf("LogText %v", i)
	}

	l.Close()

	fn := l.GetLogDirectory() + logName + ".log"
	_, err := os.Stat(fn)
	if err != nil {
		t.Errorf("Failed to find active log file.")
	}
	os.Remove(fn)

	fn = l.GetLogDirectory() + logName + ".log.1"
	_, err = os.Stat(fn)
	if err != nil {
		t.Errorf("Failed to find the 1st rotated log file.")
	}
	os.Remove(fn)

	fn = l.GetLogDirectory() + logName + ".log.2"
	_, err = os.Stat(fn)
	if err == nil {
		t.Errorf("Found the 2nd rotated log file which should have been deleted.")
	}
	os.Remove(fn)
}

func TestPid(t *testing.T) {
	logDirectory := "" // This sets the current location for logs
	l := NewLogger(logName, LevelInfo, TargetLogfile, logDirectory)
	if l == nil {
		t.Fatalf("Failed to create logger.")
	}

	l.Printf("LogText %v", 1)
	l.Close()
	fn := l.GetLogDirectory() + logName + ".log"
	defer os.Remove(fn)

	logBytes, err := os.ReadFile(fn)
	if err != nil {
		t.Fatalf("Failed to read log, %v", err)
	}
	log := string(logBytes)
	exptectedLog := fmt.Sprintf("[%v] LogText 1", os.Getpid())

	if !strings.Contains(log, exptectedLog) {
		t.Fatalf("Unexpected log: %s.", log)
	}
}
