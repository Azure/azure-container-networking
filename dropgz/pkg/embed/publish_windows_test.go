package embed

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDeployReaderRunningExecutable(t *testing.T) {
	const childEnv = "DROPGZ_TEST_RUNNING_EXECUTABLE"
	if os.Getenv(childEnv) != "" {
		fmt.Println("ready")
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "plugin.exe")
	writeFile(t, dest, old)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, dest, "-test.run=^TestDeployReaderRunningExecutable$")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := stdin.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if waitErr := cmd.Wait(); waitErr != nil {
			t.Error(waitErr)
		}
	}()
	if line, readErr := bufio.NewReader(stdout).ReadString('\n'); readErr != nil || line != "ready\n" {
		t.Fatalf("child not ready: %q, %v", line, readErr)
	}

	payload := []byte("new executable")
	if err = deployReader(dest, io.NopCloser(bytes.NewReader(payload))); err == nil {
		t.Fatal("expected replacement of the running executable to fail")
	}
	assertFile(t, dest, old)
	assertFile(t, dest+oldFileSuffix, old)
}
