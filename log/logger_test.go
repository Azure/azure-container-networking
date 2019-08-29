// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
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
	l := NewLogger(logName, LevelInfo, TargetLogfile)
	if l == nil {
		t.Fatalf("Failed to create logger.")
	}

	l.Printf("LogText %v", 1)
	l.Close()
	fn := l.GetLogDirectory() + logName + ".log"
	defer os.Remove(fn)
	
	logBytes, err := ioutil.ReadFile(fn)
	if err != nil {
		t.Fatalf("Failed to read log, %v", err)
	}
	log := string(logBytes)
	exptectedLog := fmt.Sprintf("[%v] LogText 1", os.Getpid());

	if  !strings.Contains(log, exptectedLog){
		t.Fatalf("Unexpected log: %s.", log)
	}
}
