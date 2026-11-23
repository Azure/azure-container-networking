//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
)

const (
	checkpointBefore = "before"
	checkpointAfter  = "after"
)

// npmSnapshot carries the before-checkpoint candidate identity and mounted Lite configuration to the after checkpoint.
type npmSnapshot struct {
	DaemonSetUID        string           `json:"daemonSetUID"`
	DaemonSetGeneration int64            `json:"daemonSetGeneration"`
	Identity            []podIdentity    `json:"identity"`
	Configs             []snapshotConfig `json:"configs"`
}

type snapshotConfig struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	MapName string `json:"mapName"`
	MapUID  string `json:"mapUID"`
}

func newSnapshot(before inventory, configs []mountedConfig) npmSnapshot {
	out := npmSnapshot{
		DaemonSetUID: before.daemonSet.Metadata.UID, DaemonSetGeneration: before.daemonSet.Metadata.Generation,
		Identity: before.identity, Configs: make([]snapshotConfig, 0, len(configs)),
	}
	for _, config := range configs {
		out.Configs = append(out.Configs, snapshotConfig{
			Path: config.path, Content: config.content, MapName: config.mapName, MapUID: config.mapUID,
		})
	}
	return out
}

func (s npmSnapshot) beforeInventory() inventory {
	return inventory{
		daemonSet: daemonSet{Metadata: objectMeta{UID: s.DaemonSetUID, Generation: s.DaemonSetGeneration}},
		identity:  s.Identity,
	}
}

func (s npmSnapshot) mountedConfigs() []mountedConfig {
	out := make([]mountedConfig, 0, len(s.Configs))
	for _, config := range s.Configs {
		out = append(out, mountedConfig{path: config.Path, content: config.Content, mapName: config.MapName, mapUID: config.MapUID})
	}
	return out
}

// recordCheckpoint waits, within one startup budget, for the candidate HostProcess identity and every pod's mounted
// and loaded Lite configuration, then records them for verifyCheckpoint.
func (r *runner) recordCheckpoint(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, npmStartupLimit)
	defer cancel()
	configs, err := r.startNPM(ctx)
	err = withinBudget(ctx, err)
	if err != nil {
		return err
	}
	return writeSnapshot(path, newSnapshot(r.before, configs))
}

// verifyCheckpoint observes the candidate once and rejects any identity or configuration drift since recordCheckpoint.
func (r *runner) verifyCheckpoint(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, npmStartupLimit)
	defer cancel()
	snapshot, err := readSnapshot(path)
	if err != nil {
		return err
	}
	r.before = snapshot.beforeInventory()
	configs, err := r.observeNPM(ctx)
	err = withinBudget(ctx, err)
	if err != nil {
		return err
	}
	if !slices.Equal(snapshot.mountedConfigs(), configs) {
		return fmt.Errorf("%w: mounted npm config changed since the before checkpoint", errEvidence)
	}
	return nil
}

// withinBudget fails an observation that finished after the checkpoint budget, even when it succeeded.
func withinBudget(ctx context.Context, err error) error {
	if budgetErr := ctx.Err(); budgetErr != nil {
		return errors.Join(err, fmt.Errorf("npm checkpoint budget: %w", budgetErr))
	}
	return err
}

func writeSnapshot(path string, snapshot npmSnapshot) error {
	data, err := json.Marshal(snapshot) //nolint:musttag // podIdentity is only round-tripped by this executor.
	if err != nil {
		return fmt.Errorf("encode npm snapshot: %w", err)
	}
	// Refuse to replace evidence from an earlier checkpoint.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create npm snapshot: %w", err)
	}
	_, writeErr := file.Write(data)
	if err = errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write npm snapshot: %w", err)
	}
	return nil
}

func readSnapshot(path string) (npmSnapshot, error) {
	var snapshot npmSnapshot
	info, err := os.Lstat(path)
	if err != nil {
		return snapshot, fmt.Errorf("inspect npm snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return snapshot, fmt.Errorf("%w: npm snapshot must be a private regular file", errInput)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshot, fmt.Errorf("read npm snapshot: %w", err)
	}
	err = json.Unmarshal(data, &snapshot) //nolint:musttag // podIdentity is only round-tripped by this executor.
	if err != nil {
		return snapshot, fmt.Errorf("%w: decode npm snapshot: %w", errInput, err)
	}
	// An empty identity would let verifyCheckpoint pass without observing any pod.
	if snapshot.DaemonSetUID == "" || len(snapshot.Identity) == 0 || len(snapshot.Configs) != len(snapshot.Identity) {
		return snapshot, fmt.Errorf("%w: npm snapshot lacks complete before-checkpoint evidence", errInput)
	}
	return snapshot, nil
}
