package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func cliProcess(ctx context.Context, t *testing.T, input options) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestCLISignalsCancelKubectl$", "--",
		"--kubeconfig", input.kubeconfig, "--daemonset", input.daemonSet,
		"--npm-namespace", input.npmNamespace, "--npm-image", input.image,
		"--windows-os-version", input.osVersion, "--image-manifest", input.manifest,
		"--image-config", input.config, "--source-version", input.expectedSource,
		"--run-id", strconv.FormatInt(input.runID, 10), "--output", input.output)
	cmd.Env = append(os.Environ(), "ACN_NPM_LITE_SIGNAL_TEST=1")
	cmd.WaitDelay = time.Second
	return cmd
}

func TestCLISignalsCancelKubectl(t *testing.T) {
	if os.Getenv("ACN_NPM_LITE_SIGNAL_TEST") == "1" {
		start := slices.Index(os.Args, "--")
		if start < 0 {
			t.Fatal("missing subprocess inputs")
		}
		os.Args = append([]string{"windows-npm-lite"}, os.Args[start+1:]...)
		sourceVersion = strings.Repeat("c", 40)
		main()
		t.Fatal("interrupted CLI returned success")
	}
	for _, interrupt := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(interrupt.String(), func(t *testing.T) {
			_, input := testCluster(t)
			directory := t.TempDir()
			pidFile := filepath.Join(directory, "kubectl.pid")
			script := "#!/bin/sh\nsleep 300 &\nchild=$!\nprintf '%s %s\\n' \"$$\" \"$child\" > \"$ACN_NPM_LITE_PID\"\nwait \"$child\"\n"
			if err := os.WriteFile(filepath.Join(directory, "kubectl"), []byte(script), 0o700); err != nil { //nolint:gosec // Owner-only executable test fixture.
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			cmd := cliProcess(ctx, t, input)
			cmd.Env = append(cmd.Env,
				"ACN_NPM_LITE_PID="+pidFile, "PATH="+directory+":"+os.Getenv("PATH"))
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			var pid, descendant int
			for pid == 0 {
				raw, readErr := os.ReadFile(pidFile)
				if readErr == nil && len(bytes.TrimSpace(raw)) != 0 {
					if _, err := fmt.Sscanf(string(raw), "%d %d", &pid, &descendant); err != nil || pid <= 0 || descendant <= 0 {
						t.Fatalf("invalid subprocess PIDs: %q, %v", raw, err)
					}
					break
				}
				if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
					t.Fatal(readErr)
				}
				select {
				case <-ctx.Done():
					t.Fatal("kubectl subprocess did not start")
				case <-ticker.C:
				}
			}
			child, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if child.Signal(syscall.Signal(0)) == nil {
					_ = child.Kill()
				}
				_ = child.Release()
			})
			if signalErr := cmd.Process.Signal(interrupt); signalErr != nil {
				t.Fatal(signalErr)
			}
			var exitErr *exec.ExitError
			if waitErr := cmd.Wait(); !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 1 || ctx.Err() != nil {
				t.Fatalf("signal exit: %v, output: %s", waitErr, output.String())
			}
			if !strings.Contains(output.String(), "context canceled") || child.Signal(syscall.Signal(0)) == nil {
				t.Fatalf("signal did not cancel and reap kubectl: %s", output.String())
			}
			stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", descendant))
			if err == nil && !strings.Contains(string(stat), ") Z ") {
				_ = syscall.Kill(descendant, syscall.SIGKILL)
				t.Fatal("kubectl credential-helper descendant survived cancellation")
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			if _, err := os.Stat(input.output); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("interrupted CLI published a result: %v", err)
			}
		})
	}
}

func TestMetadataFIFOIsRejectedBeforeOpening(t *testing.T) {
	for _, field := range []string{"child manifest", "image configuration"} {
		t.Run(field, func(t *testing.T) {
			_, input := testCluster(t)
			path := filepath.Join(t.TempDir(), "metadata.fifo")
			if err := syscall.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
			if field == "child manifest" {
				input.manifest = path
			} else {
				input.config = path
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			output, err := cliProcess(ctx, t, input).CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || ctx.Err() != nil ||
				!bytes.Contains(output, []byte("metadata input must be a regular file")) {
				t.Fatalf("FIFO preflight: %v, output: %s", err, output)
			}
		})
	}
}
