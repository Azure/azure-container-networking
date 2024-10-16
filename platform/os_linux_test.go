package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
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

func TestExecuteCommand(t *testing.T) {
	output, err := NewExecClient(nil).ExecuteCommand(context.Background(), "echo", "/B && echo two")
	if err != nil {
		t.Errorf("TestExecuteCommand failed with error %v", err)
	}
	if strings.TrimRight(output, "\n\r") != "/B && echo two" {
		t.Errorf("TestExecuteCommand failed with output %s", output)
	}
}

func TestExecuteCommandError(t *testing.T) {
	_, err := NewExecClient(nil).ExecuteCommand(context.Background(), "donotaddtopath")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("TestExecuteCommand failed with error %v", err)
	}
}

// Command execution time is more than timeout, so ExecuteCommand should return error
func TestExecuteCommandTimeout(t *testing.T) {
	const timeout = 2 * time.Second
	client := NewExecClientTimeout(timeout)

	_, err := client.ExecuteCommand(context.Background(), "sleep", "3")
	if err == nil {
		t.Errorf("TestExecuteCommandTimeout should have returned timeout error")
	}
	t.Logf("%s", err.Error())
}
