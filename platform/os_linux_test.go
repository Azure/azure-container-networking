package platform

import (
	"testing"
	"time"
)

// Command execution time is more than timeout, so ExecuteRawCommand should return error
func TestExecuteRawCommandTimeout(t *testing.T) {
	const timeout = 2 * time.Second
	client := NewExecClientTimeout(timeout)

	_, err := client.ExecuteRawCommand("sleep 3")
	if err == nil {
		t.Errorf("TestExecuteRawCommandTimeout should have returned timeout error")
	}
	t.Logf("%s", err.Error())
}

// Command execution time is less than timeout, so ExecuteRawCommand should work without error
func TestExecuteRawCommandNoTimeout(t *testing.T) {
	const timeout = 2 * time.Second
	client := NewExecClientTimeout(timeout)

	_, err := client.ExecuteRawCommand("sleep 1")
	if err != nil {
		t.Errorf("TestExecuteRawCommandNoTimeout failed with error %v", err)
	}
}
