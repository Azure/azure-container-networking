//go:build linux || darwin

package embed

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDeployReaderWriteFailure(t *testing.T) {
	const childEnv = "DROPGZ_TEST_WRITE_FAILURE"
	if os.Getenv(childEnv) == "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestDeployReaderWriteFailure$")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("write-failure subprocess: %v\n%s", err, output)
		}
		return
	}

	// Limit only this subprocess, not the test runner or other deployments.
	signal.Ignore(syscall.SIGXFSZ)
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	limit.Cur = 1024
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin")
	old := []byte("previous executable")
	backup := []byte("earlier executable")
	writeFile(t, dest, old)
	writeFile(t, dest+oldFileSuffix, backup)
	rc := &compoundReadCloser{readcloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("payload"), 8192)))}
	if err := deployReader(dest, rc); !errors.Is(err, syscall.EFBIG) {
		t.Fatalf("expected file size limit error, got %v", err)
	}
	assertFile(t, dest, old)
	assertFile(t, dest+oldFileSuffix, backup)
	assertNoTemps(t, dir)
}
