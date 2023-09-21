// Copyright 2017 Microsoft. All rights reserved.
// MIT License

package store

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-container-networking/cni/log"
	"github.com/Azure/azure-container-networking/processlock"
	"github.com/stretchr/testify/require"
)

const (
	// File name used for test store.
	testFileName     = "test.json"
	testLockFileName = "locktest.json"
	// Keys used during tests.
	testKey1 = "key1"
	testKey2 = "key2"
)

// Type for testing aggregate encoding.
type testType1 struct {
	Field1 string
	Field2 int
}

// Tests that the key value pairs are reinstantiated correctly from a pre-existing JSON encoded file.
func TestKeyValuePairsAreReinstantiatedFromJSONFile(t *testing.T) {
	encodedPair := `{"key1":{"Field1":"test","Field2":42}}`
	expectedValue := testType1{"test", 42}
	var actualValue testType1

	// Create a JSON file containing the encoded pair.
	file, err := os.Create(testFileName)
	if err != nil {
		t.Fatalf("Failed to create file %v", err)
	}

	_, err = file.WriteString(encodedPair)
	if err != nil {
		t.Fatalf("Failed to write to file %v", err)
	}

	file.Close()
	defer os.Remove(testFileName)

	// Create the store, initialized using the JSON file.
	kvs, err := NewJsonFileStore(testFileName, processlock.NewMockFileLock(false), nil)
	if err != nil {
		t.Fatalf("Failed to create KeyValueStore %v\n", err)
	}

	// Read the pair.
	err = kvs.Read(testKey1, &actualValue)
	if err != nil {
		t.Fatalf("Failed to read from store %v", err)
	}

	// Fail if the read pair does not match the expected pair.
	if actualValue != expectedValue {
		t.Errorf("Read pair (%v, %v) does not match the expected pair (%v, %v)",
			testKey1, actualValue, testKey1, expectedValue)
	}
}

// Tests that the key value pairs written to the store are persisted correctly in JSON encoded file.
func TestKeyValuePairsArePersistedToJSONFile(t *testing.T) {
	writtenValue := testType1{"test", 42}
	expectedPair := `{"key1":{"Field1":"test","Field2":42}}`
	var actualPair string

	// Create the store.
	kvs, err := NewJsonFileStore(testFileName, processlock.NewMockFileLock(false), nil)
	if err != nil {
		t.Fatalf("Failed to create KeyValueStore %v\n", err)
	}

	// Write the value.
	err = kvs.Write(testKey1, &writtenValue)
	if err != nil {
		t.Fatalf("Failed to write to store %v", err)
	}

	// Read the persisted file contents.
	file, err := os.Open(testFileName)
	if err != nil {
		t.Fatalf("Failed to open file %v", err)
	}

	data := make([]byte, 100)
	n, err := file.Read(data)
	if err != nil {
		t.Fatalf("Failed to read from file %v", err)
	}

	file.Close()
	os.Remove(testFileName)

	// Remove indentation to normalize the JSON encoding.
	actualPair = string(data[:n])
	actualPair = strings.Replace(actualPair, " ", "", -1)
	actualPair = strings.Replace(actualPair, "\t", "", -1)
	actualPair = strings.Replace(actualPair, "\n", "", -1)

	// Fail if the contents do not match expected JSON encoding.
	if actualPair != expectedPair {
		t.Errorf("Read pair (%v, %v) does not match the expected pair (%v, %v)",
			testKey1, actualPair, testKey1, expectedPair)
	}
}

// Tests that key value pairs are written and read back correctly.
func TestKeyValuePairsAreWrittenAndReadCorrectly(t *testing.T) {
	writtenValue := testType1{"test", 42}
	anotherValue := testType1{"any", 14}
	var readValue testType1

	// Test when passing zap logger obj to NewJsonFileStore
	logger := log.CNILogger

	// Create the store.
	kvs, err := NewJsonFileStore(testFileName, processlock.NewMockFileLock(false), logger)
	if err != nil {
		t.Fatalf("Failed to create KeyValueStore %v\n", err)
	}

	// Write a key value pair.
	err = kvs.Write(testKey1, &writtenValue)
	if err != nil {
		t.Fatalf("Failed to write to store %v", err)
	}

	// Write a second key value pair.
	err = kvs.Write(testKey2, &anotherValue)
	if err != nil {
		t.Fatalf("Failed to write to store %v", err)
	}

	// Read the first pair back.
	err = kvs.Read(testKey1, &readValue)
	if err != nil {
		t.Fatalf("Failed to read from store %v", err)
	}

	// Fail if the read pair does not match the written pair.
	if readValue != writtenValue {
		t.Errorf("Read pair (%v, %v) does not match the written pair (%v, %v)",
			testKey1, readValue, testKey1, writtenValue)
	}

	// Cleanup.
	os.Remove(testFileName)
}

// test case for testing newjsonfilestore idempotent
func TestNewJsonFileStoreIdempotent(t *testing.T) {
	_, err := NewJsonFileStore(testLockFileName, processlock.NewMockFileLock(false), nil)
	if err != nil {
		t.Errorf("Failed to initialize store: %v", err)
	}

	_, err = NewJsonFileStore(testLockFileName, processlock.NewMockFileLock(false), nil)
	if err != nil {
		t.Errorf("Failed to initialize same store second time: %v", err)
	}
}

func TestLock(t *testing.T) {
	tests := []struct {
		name       string
		store      KeyValueStore
		timeoutms  int
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "Acquire Lock happy path",
			store: func() KeyValueStore {
				st, _ := NewJsonFileStore(testFileName, processlock.NewMockFileLock(false), nil)
				return st
			}(),
			timeoutms: 10000,
			wantErr:   false,
		},
		{
			name: "Acquire Lock Fail",
			store: func() KeyValueStore {
				st, _ := NewJsonFileStore(testFileName, processlock.NewMockFileLock(true), nil)
				return st
			}(),
			timeoutms:  10000,
			wantErr:    true,
			wantErrMsg: processlock.ErrMockFileLock.Error(),
		},
		{
			name: "Acquire Lock timeout error",
			store: func() KeyValueStore {
				st, _ := NewJsonFileStore(testFileName, processlock.NewMockFileLock(false), nil)
				return st
			}(),
			timeoutms:  0,
			wantErr:    true,
			wantErrMsg: ErrTimeoutLockingStore.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.store.Lock(time.Duration(tt.timeoutms) * time.Millisecond)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrMsg, "Expected:%v but got:%v", tt.wantErrMsg, err.Error())
			} else {
				require.NoError(t, err)
				err = tt.store.Unlock()
				require.NoError(t, err)
			}
		})
	}
}

// test case for testing newjsonfilestore idempotent
func TestFileName(t *testing.T) {
	_, err := NewJsonFileStore("", processlock.NewMockFileLock(false), nil)
	if err == nil {
		t.Errorf("This should have failed for empty file name")
	}

	_, err = NewJsonFileStore("test.json", processlock.NewMockFileLock(false), nil)
	if err != nil {
		t.Fatalf("This should not fail for a non-empty file %v", err)
	}
}
