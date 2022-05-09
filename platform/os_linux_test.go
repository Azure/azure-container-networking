package platform

import (
	"testing"
	"time"
)

// Command execution time is more than timeout, so ExecuteCommand should return error
func TestExecuteCommandTimeout(t *testing.T) {
	const timeout = 2 * time.Second
	client := NewExecClientTimeout(timeout)

	_, err := client.ExecuteCommand("sleep 3")
	if err == nil {
		t.Errorf("TestExecuteCommandTimeout should have returned timeout error")
	}
	t.Logf("%s", err.Error())
}

// Command execution time is less than timeout, so ExecuteCommand should work without error
func TestExecuteCommandNoTimeout(t *testing.T) {
	const timeout = 2 * time.Second
	client := NewExecClientTimeout(timeout)

	_, err := client.ExecuteCommand("sleep 1")
	if err != nil {
		t.Errorf("TestExecuteCommandNoTimeout failed with error %v", err)
	}
}
