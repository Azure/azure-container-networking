// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package log

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	logName = "test"
)

func TestToJSONString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: "null",
		},
		{
			name:     "simple struct",
			input:    struct{ Name string }{"test"},
			expected: `{"Name":"test"}`,
		},
		{
			name: "struct with json.RawMessage",
			input: struct {
				ID      string
				Data    json.RawMessage
				Enabled bool
			}{
				ID:      "123",
				Data:    json.RawMessage(`{"foo":"bar"}`),
				Enabled: true,
			},
			expected: `{"ID":"123","Data":{"foo":"bar"},"Enabled":true}`,
		},
		{
			name: "nested json.RawMessage",
			input: struct {
				Context json.RawMessage
			}{
				Context: json.RawMessage(`{"PodName":"test-pod","PodNamespace":"default"}`),
			},
			expected: `{"Context":{"PodName":"test-pod","PodNamespace":"default"}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ToJSONString(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestToJSONStringFallback(t *testing.T) {
	// Create a circular reference that cannot be marshaled to JSON
	type Circular struct {
		Name     string
		Referrer *Circular
	}

	circular := &Circular{Name: "test"}
	circular.Referrer = circular // Create circular reference

	// This should fall back to %+v formatting
	result := ToJSONString(circular)
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "Referrer")
}

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

func TestRotateFailure(t *testing.T) {
	// this test simulates a scenario where log rotation should fail because the file to rotate does not exist.
	// previously, such a scenario would recursively call Logf, which would deadlock because of the mutexes taken.
	// the logging api does not bubble up any errors, and does not allow any configuration of the frequency of
	// rotation checks, but we shouldn't couple this test (too much) to the internal algorithm.
	//
	// the assertions below should demonstrate that:
	// - logging does not block even if the target file is missing.
	// - an explicit rotation call should return with an error and not block indefinitely.
	// - successive calls after a rotation failure should not block either.
	l := NewLogger(logName, LevelInfo, TargetStdOutAndLogFile, "/tmp/")
	require.NotNil(t, l)

	err := os.Remove(l.getLogFileName())
	require.NoError(t, err)

	l.Logf("this log line may or may not invoke rotation")

	err = l.rotate()
	var pErr *os.PathError
	assert.ErrorAs(t, err, &pErr)

	l.Logf("this log line may or may not invoke rotation")
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
