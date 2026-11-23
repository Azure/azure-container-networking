//go:build linux

//nolint:goconst // Keep wire fixtures independent of executor constants.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

var foreignImageID = "sha256:" + strings.Repeat("b", 64)

func runCheckpoint(t *testing.T, input options, command commandFunc, checkpoint, snapshot string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Minute)
	defer cancel()
	input.checkpoint, input.snapshot = checkpoint, snapshot
	return run(ctx, input, command)
}

func requireNoSnapshot(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed before checkpoint recorded a snapshot: %v", err)
	}
}

func writePrivate(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // Owner-only test fixture under t.TempDir.
		t.Fatal(err)
	}
}

func fullNPMConfig(f *fakeCluster) {
	f.config.Data["azure-npm.json"] = strings.Replace(f.config.Data["azure-npm.json"], `"EnableNPMLite": true`, `"EnableNPMLite": false`, 1)
}

func withoutBootupMarker(result commandResult, err error) (commandResult, error) {
	result.stdout = []byte(strings.ReplaceAll(string(result.stdout), "finished bootup phase", "bootup still pending"))
	return result, err
}

func TestCheckpointsVerifyCandidateWithoutCreatingWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cluster, input := testCluster(t)
		snapshot := filepath.Join(t.TempDir(), "npm-snapshot.json")
		if err := runCheckpoint(t, input, cluster.command, checkpointBefore, snapshot); err != nil {
			t.Fatal(err)
		}
		info, statErr := os.Lstat(snapshot)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("snapshot is not a private regular file: %v", statErr)
		}
		if err := runCheckpoint(t, input, cluster.command, checkpointAfter, snapshot); err != nil {
			t.Fatal(err)
		}
		if cluster.policiesCreated != 0 || cluster.probes != 0 || cluster.namespace.Metadata.UID != "" || len(cluster.roles) != 0 {
			t.Fatal("checkpoints created conformance work")
		}
		if err := runCheckpoint(t, input, cluster.command, checkpointBefore, snapshot); !errors.Is(err, os.ErrExist) {
			t.Fatalf("replaced earlier checkpoint evidence: %v", err)
		}
	})
}

func TestBeforeCheckpointRejectsNonCandidateNPM(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*fakeCluster)
	}{
		{"mounted full npm config", fullNPMConfig},
		{"loaded full npm config", func(f *fakeCluster) { f.fault = "loaded full npm config" }},
		{"running image", func(f *fakeCluster) { f.npm[1].Status.ContainerStatuses[0].ImageID = foreignImageID }},
		{"not HostProcess", func(f *fakeCluster) {
			hostProcess := false
			f.npm[0].Spec.SecurityContext.WindowsOptions.HostProcess = &hostProcess
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cluster, input := testCluster(t)
				test.edit(cluster)
				snapshot := filepath.Join(t.TempDir(), "npm-snapshot.json")
				start := time.Now()
				err := runCheckpoint(t, input, cluster.command, checkpointBefore, snapshot)
				if !errors.Is(err, errEvidence) || time.Since(start) != 0 {
					t.Fatalf("before checkpoint accepted or awaited non-candidate npm for %s: %v", time.Since(start), err)
				}
				requireNoSnapshot(t, snapshot)
			})
		})
	}
}

func TestAfterCheckpointRejectsDriftWithoutRetry(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*fakeCluster)
		want error
	}{
		{"container restart", func(f *fakeCluster) { f.npm[0].Status.ContainerStatuses[0].RestartCount++ }, errEvidence},
		{"daemonset generation", func(f *fakeCluster) {
			f.ds.Metadata.Generation++
			f.ds.Status.ObservedGeneration++
		}, errEvidence},
		{"running image", func(f *fakeCluster) { f.npm[1].Status.ContainerStatuses[0].ImageID = foreignImageID }, errEvidence},
		{"mounted config content", func(f *fakeCluster) { f.config.Data["azure-npm.json"] += "\n" }, errEvidence},
		{"replaced configmap", func(f *fakeCluster) { f.config.Metadata.UID = "replacement-uid" }, errEvidence},
		{"mounted full npm config", fullNPMConfig, errEvidence},
		{"loaded full npm config", func(f *fakeCluster) { f.fault = "loaded full npm config" }, errEvidence},
		{"rollout not ready", func(f *fakeCluster) { f.ds.Status.Ready = 1 }, errNotReady},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cluster, input := testCluster(t)
				snapshot := filepath.Join(t.TempDir(), "npm-snapshot.json")
				if err := runCheckpoint(t, input, cluster.command, checkpointBefore, snapshot); err != nil {
					t.Fatal(err)
				}
				test.edit(cluster)
				start := time.Now()
				err := runCheckpoint(t, input, cluster.command, checkpointAfter, snapshot)
				if !errors.Is(err, test.want) || time.Since(start) != 0 {
					t.Fatalf("after checkpoint accepted or retried drift for %s: %v", time.Since(start), err)
				}
			})
		})
	}
}

func TestBeforeCheckpointSharesOneStartupBudget(t *testing.T) {
	for _, test := range []struct {
		name string
		wrap func(*fakeCluster, time.Time) commandFunc
	}{
		{"pod discovery then pending bootup", func(f *fakeCluster, start time.Time) commandFunc {
			all := f.npm
			return func(ctx context.Context, args []string, stdin []byte) (commandResult, error) {
				f.npm = all
				if time.Since(start) < 80*time.Second {
					f.npm = all[:1]
				}
				if slices.Contains(args, "logs") {
					return withoutBootupMarker(f.command(ctx, args, stdin))
				}
				return f.command(ctx, args, stdin)
			}
		}},
		{"slow log reads", func(f *fakeCluster, _ time.Time) commandFunc {
			return func(ctx context.Context, args []string, stdin []byte) (commandResult, error) {
				if !slices.Contains(args, "logs") {
					return f.command(ctx, args, stdin)
				}
				timer := time.NewTimer(30 * time.Second)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return commandResult{}, fmt.Errorf("slow log read: %w", ctx.Err())
				case <-timer.C:
				}
				if slices.Contains(args, f.npm[1].Metadata.Name) {
					return withoutBootupMarker(f.command(ctx, args, stdin))
				}
				return f.command(ctx, args, stdin)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cluster, input := testCluster(t)
				snapshot := filepath.Join(t.TempDir(), "npm-snapshot.json")
				start := time.Now()
				err := runCheckpoint(t, input, test.wrap(cluster, start), checkpointBefore, snapshot)
				if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, errNotReady) || time.Since(start) != npmStartupLimit {
					t.Fatalf("startup budget: elapsed %s, error %v", time.Since(start), err)
				}
				requireNoSnapshot(t, snapshot)
			})
		})
	}
}

func TestCheckpointsRejectLateSuccess(t *testing.T) {
	for _, checkpoint := range []string{checkpointBefore, checkpointAfter} {
		t.Run(checkpoint, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cluster, input := testCluster(t)
				snapshot := filepath.Join(t.TempDir(), "npm-snapshot.json")
				if checkpoint == checkpointAfter {
					if err := runCheckpoint(t, input, cluster.command, checkpointBefore, snapshot); err != nil {
						t.Fatal(err)
					}
				}
				// Log reads ignore cancellation, so the second pod's read succeeds 20s after the budget.
				command := func(ctx context.Context, args []string, stdin []byte) (commandResult, error) {
					if slices.Contains(args, "logs") {
						time.Sleep(160 * time.Second)
					}
					return cluster.command(context.WithoutCancel(ctx), args, stdin)
				}
				start := time.Now()
				err := runCheckpoint(t, input, command, checkpoint, snapshot)
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != npmStartupLimit+20*time.Second {
					t.Fatalf("accepted late %s checkpoint after %s: %v", checkpoint, time.Since(start), err)
				}
				if checkpoint == checkpointBefore {
					requireNoSnapshot(t, snapshot)
				}
			})
		})
	}
}

func TestAfterCheckpointRejectsUntrustedSnapshots(t *testing.T) {
	edited := func(edit func(*npmSnapshot)) func(*testing.T, []byte, string) {
		return func(t *testing.T, valid []byte, path string) {
			t.Helper()
			snapshot := decodeTest[npmSnapshot](t, valid)
			edit(&snapshot)
			writePrivate(t, path, marshalTest(t, snapshot))
		}
	}
	for _, test := range []struct {
		name    string
		prepare func(*testing.T, []byte, string)
		want    error
	}{
		{"missing", func(*testing.T, []byte, string) {}, os.ErrNotExist},
		{"group readable", func(t *testing.T, valid []byte, path string) {
			writePrivate(t, path, valid)
			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatal(err)
			}
		}, errInput},
		{"symlink", func(t *testing.T, valid []byte, path string) {
			writePrivate(t, path+".target", valid)
			if err := os.Symlink(path+".target", path); err != nil {
				t.Fatal(err)
			}
		}, errInput},
		{"directory", func(t *testing.T, _ []byte, path string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, errInput},
		{"malformed", func(t *testing.T, valid []byte, path string) { writePrivate(t, path, valid[:len(valid)-1]) }, errInput},
		{"empty identity", edited(func(s *npmSnapshot) { s.Identity, s.Configs = nil, nil }), errInput},
		{"missing config", edited(func(s *npmSnapshot) { s.Configs = s.Configs[:1] }), errInput},
		{"missing daemonset", edited(func(s *npmSnapshot) { s.DaemonSetUID = "" }), errInput},
		{"omitted candidate pod", edited(func(s *npmSnapshot) { s.Identity, s.Configs = s.Identity[:1], s.Configs[:1] }), errEvidence},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cluster, input := testCluster(t)
				dir := t.TempDir()
				valid := filepath.Join(dir, "valid.json")
				if err := runCheckpoint(t, input, cluster.command, checkpointBefore, valid); err != nil {
					t.Fatal(err)
				}
				data, readErr := os.ReadFile(valid)
				if readErr != nil {
					t.Fatal(readErr)
				}
				snapshot := filepath.Join(dir, "npm-snapshot.json")
				test.prepare(t, data, snapshot)
				if err := runCheckpoint(t, input, cluster.command, checkpointAfter, snapshot); !errors.Is(err, test.want) {
					t.Fatalf("after checkpoint trusted the snapshot: %v", err)
				}
			})
		})
	}
}

func TestCLICheckpointRequiresSnapshot(t *testing.T) {
	image := testImage(t)
	args := []string{"--kubeconfig", "private-kubeconfig", "--daemonset", "azure-npm-win", "--npm-image", image.Reference}
	for _, checkpoint := range []string{checkpointBefore, checkpointAfter} {
		input, err := parseOptions(append(slices.Clone(args), "--checkpoint", checkpoint, "--snapshot", "npm-snapshot.json"))
		if err != nil || input.checkpoint != checkpoint || input.snapshot != "npm-snapshot.json" {
			t.Fatalf("%s checkpoint: %+v, %v", checkpoint, input, err)
		}
	}
	for _, extra := range [][]string{
		{"--checkpoint", checkpointBefore},
		{"--snapshot", "npm-snapshot.json"},
		{"--checkpoint", "during", "--snapshot", "npm-snapshot.json"},
	} {
		if _, err := parseOptions(append(slices.Clone(args), extra...)); !errors.Is(err, errInput) {
			t.Fatalf("accepted checkpoint input %v: %v", extra, err)
		}
	}
}
