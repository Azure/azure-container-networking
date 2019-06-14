package platform

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestGetLastRebootTime(t *testing.T) {
	_, err := GetLastRebootTime()
	if err != nil {
		t.Errorf("GetLastRebootTime failed :%v", err)
	}
}

func TestGetOSDetails(t *testing.T) {
	_, err := GetOSDetails()
	if err != nil {
		t.Errorf("GetOSDetails failed :%v", err)
	}
}
