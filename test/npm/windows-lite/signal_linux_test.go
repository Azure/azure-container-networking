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
		"--npm-namespace", input.npmNamespace, "--npm-image", input.image)
	cmd.Env = append(os.Environ(), "ACN_NPM_LITE_SIGNAL_TEST=1")
	cmd.WaitDelay = time.Second
	return cmd
}

func cleanupFixtureProcessGroup(t *testing.T, pid int) {
	t.Helper()
	t.Cleanup(func() {
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			t.Errorf("terminate fixture process group: %v", err)
		}
	})
}

func assertFixtureProcessExited(t *testing.T, pid int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) ||
			(err == nil && strings.Contains(string(stat), ") Z ")) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("fixture process %d survived teardown", pid)
		case <-ticker.C:
		}
	}
}

func TestKubectlExitCleansProcessGroup(t *testing.T) {
	for _, test := range []struct {
		name     string
		redirect string
		exitCode int
	}{
		{name: "inherited-pipes"},
		{name: "closed-pipes", redirect: " >/dev/null 2>&1 </dev/null"},
		{name: "failed-command", exitCode: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			pidFile := filepath.Join(directory, "kubectl.pid")
			t.Setenv("PATH", directory+":"+os.Getenv("PATH"))
			t.Setenv("ACN_NPM_LITE_PID", pidFile)
			script := fmt.Sprintf("#!/bin/sh\nsleep 300%s &\nprintf '%%s %%s\\n' \"$$\" \"$!\" > \"$ACN_NPM_LITE_PID\"\nexit %d\n",
				test.redirect, test.exitCode)
			if err := os.WriteFile(filepath.Join(directory, "kubectl"), []byte(script), 0o700); err != nil { //nolint:gosec // Owner-only executable test fixture.
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			result, runErr := invokeKubectl(ctx, nil, nil)
			raw, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatal(err)
			}
			var pid, descendant int
			if _, scanErr := fmt.Sscanf(string(raw), "%d %d", &pid, &descendant); scanErr != nil || pid <= 0 || descendant <= 0 {
				t.Fatalf("invalid fixture PIDs: %q, %v", raw, scanErr)
			}
			cleanupFixtureProcessGroup(t, pid)
			assertFixtureProcessExited(t, descendant)
			if runErr != nil || result.code != test.exitCode {
				t.Fatalf("command result changed: code %d, error %v", result.code, runErr)
			}
		})
	}
}

func TestWaitWithoutReapingPreservesExitStatus(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "/bin/sh", "-c", "exit 7")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	if err := waitWithoutReaping(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("group leader PID was released before teardown: %v", err)
	}
	var exitErr *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("exit status was reaped before Wait: %v", err)
	}
}

func TestWaitWithoutReapingRejectsNonChild(t *testing.T) {
	if err := waitWithoutReaping(os.Getpid()); !errors.Is(err, syscall.ECHILD) {
		t.Fatalf("non-child observation: %v", err)
	}
}

func TestCLISignalsCancelKubectl(t *testing.T) {
	if os.Getenv("ACN_NPM_LITE_SIGNAL_TEST") == "1" {
		start := slices.Index(os.Args, "--")
		if start < 0 {
			t.Fatal("missing subprocess inputs")
		}
		os.Args = append([]string{"windows-npm-lite"}, os.Args[start+1:]...)
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
			cleanupFixtureProcessGroup(t, pid)
			child, err := os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
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
			assertFixtureProcessExited(t, descendant)
		})
	}
}

func TestFixtureCleanupAfterFatal(t *testing.T) {
	if pidFile := os.Getenv("ACN_NPM_LITE_CLEANUP_TEST"); pidFile != "" {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
			`sleep 300 & printf '%s %s\n' "$$" "$!" > "$ACN_NPM_LITE_CLEANUP_TEST"`)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cleanupFixtureProcessGroup(t, cmd.Process.Pid)
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("intentional failure after fixture leader exited")
	}
	pidFile := filepath.Join(t.TempDir(), "fixture.pid")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestFixtureCleanupAfterFatal$")
	cmd.Env = append(os.Environ(), "ACN_NPM_LITE_CLEANUP_TEST="+pidFile)
	cmd.WaitDelay = time.Second
	output, runErr := cmd.CombinedOutput()
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("fixture PIDs: %v, output: %s", err, output)
	}
	var pid, descendant int
	if _, scanErr := fmt.Sscanf(string(raw), "%d %d", &pid, &descendant); scanErr != nil || pid <= 0 || descendant <= 0 {
		t.Fatalf("invalid fixture PIDs: %q, %v", raw, scanErr)
	}
	cleanupFixtureProcessGroup(t, pid)
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 || ctx.Err() != nil ||
		!bytes.Contains(output, []byte("intentional failure after fixture leader exited")) {
		t.Fatalf("fixture failure: %v, output: %s", runErr, output)
	}
	assertFixtureProcessExited(t, descendant)
}
