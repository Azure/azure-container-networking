package common

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestReadFileByLines(t *testing.T) {
	lines, err := ReadFileByLines("testfiles/test1")
	if err != nil {
		t.Errorf("ReadFileByLines failed: %v", err)
	}

	if len(lines) != 2 {
		t.Errorf("Line count %d didn't match expected count", len(lines))
	}

	lines = nil

	lines, err = ReadFileByLines("testfiles/test2")
	if err != nil {
		t.Errorf("ReadFileByLines failed: %v", err)
	}

	if len(lines) != 1 {
		t.Errorf("Line count %d didn't match expected count", len(lines))
	}

	lines = nil

	lines, err = ReadFileByLines("testfiles/test3")
	if err != nil {
		t.Errorf("ReadFileByLines failed: %v", err)
	}

	if len(lines) != 2 {
		t.Errorf("Line count %d didn't match expected count", len(lines))
	}

	if lines[1] != "" {
		t.Errorf("Expected empty line but got %s", lines[1])
	}
}

func TestFileExists(t *testing.T) {
	isExist, err := CheckIfFileExists("testfiles/test1")
	if err != nil || !isExist {
		t.Errorf("Returned file not found %v", err)
	}

	isExist, err = CheckIfFileExists("testfiles/filenotfound")
	if err != nil || isExist {
		t.Errorf("Returned file found")
	}
}
