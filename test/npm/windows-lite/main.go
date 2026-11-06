//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

var (
	sourceVersion string
	errInput      = errors.New("windows-lite: invalid runner input")
)

type options struct {
	kubeconfig     string
	daemonSet      string
	npmNamespace   string
	image          string
	osVersion      string
	manifest       string
	config         string
	expectedSource string
	runID          int64
	output         string
}

func parseOptions(args []string, builtSource string) (options, error) {
	var out options
	flags := flag.NewFlagSet(suiteName, flag.ContinueOnError)
	flags.StringVar(&out.kubeconfig, "kubeconfig", "", "existing private kubeconfig")
	flags.StringVar(&out.daemonSet, "daemonset", "", "explicit Windows NPM DaemonSet name")
	flags.StringVar(&out.npmNamespace, "npm-namespace", "kube-system", "Windows NPM namespace")
	flags.StringVar(&out.image, "npm-image", "", "complete immutable candidate child reference")
	flags.StringVar(&out.osVersion, "windows-os-version", "", "exact Windows2022 image OS version")
	flags.StringVar(&out.manifest, "image-manifest", "", "raw candidate child manifest file")
	flags.StringVar(&out.config, "image-config", "", "raw candidate image config file")
	flags.StringVar(&out.expectedSource, "source-version", "", "expected full executable source commit")
	flags.Int64Var(&out.runID, "run-id", 0, "actual positive pipeline build ID")
	flags.StringVar(&out.output, "output", "", "new result JSON file, written only after successful cleanup")
	if err := flags.Parse(args); err != nil {
		return out, fmt.Errorf("parse flags: %w", err)
	}
	name := regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
	if flags.NArg() != 0 || !name.MatchString(out.daemonSet) || len(out.daemonSet) > 63 ||
		!name.MatchString(out.npmNamespace) || len(out.npmNamespace) > 63 ||
		out.kubeconfig == "" || out.manifest == "" || out.config == "" || out.output == "" || out.runID <= 0 {
		return out, fmt.Errorf("%w: all runner inputs and a positive run-id are required", errInput)
	}
	if !sourcePattern.MatchString(out.expectedSource) || builtSource != out.expectedSource {
		return out, fmt.Errorf("%w: executable source pin differs from expected source-version", errInput)
	}
	return out, nil
}

func readInput(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect input: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: metadata input must be a regular file", errInput)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}
	data, readErr := readBounded(file)
	return data, errors.Join(readErr, file.Close())
}

func writeResult(path string, value runResult) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create result: %w", err)
	}
	_, writeErr := file.Write(append(data, '\n'))
	err = errors.Join(writeErr, file.Sync(), file.Close())
	if err != nil {
		return errors.Join(err, os.Remove(path))
	}
	return nil
}

func run(ctx context.Context, input options, command commandFunc) error {
	if _, err := os.Lstat(input.output); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("%w: result path already exists", errInput)
		}
		return fmt.Errorf("inspect result path: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(input.output))
	if err != nil {
		return fmt.Errorf("inspect result parent: %w", err)
	}
	if !parent.IsDir() {
		return fmt.Errorf("%w: result parent is not a directory", errInput)
	}
	kubeconfig, err := os.Lstat(input.kubeconfig)
	if err != nil {
		return fmt.Errorf("inspect kubeconfig: %w", err)
	}
	if !kubeconfig.Mode().IsRegular() || kubeconfig.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: kubeconfig must be a private regular file", errInput)
	}
	manifest, err := readInput(input.manifest)
	if err != nil {
		return err
	}
	config, err := readInput(input.config)
	if err != nil {
		return err
	}
	image, err := parseImage(input.image, input.osVersion, manifest, config)
	if err != nil {
		return err
	}
	cases, err := loadCorpus()
	if err != nil {
		return err
	}
	codeHash, err := runnerHash()
	if err != nil {
		return err
	}
	r := runner{
		kube: kubeClient{input.kubeconfig, command}, npmNamespace: input.npmNamespace,
		daemonSet: input.daemonSet, image: image, corpus: cases,
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return fmt.Errorf("%w: runner requires an overall deadline", errInput)
	}
	// Reserve the final convergence window for UID-bound namespace cleanup.
	work, cancel := context.WithDeadline(ctx, deadline.Add(-convergenceLimit))
	result, executionErr := r.execute(work, input.runID, input.expectedSource, codeHash)
	cancel()
	cleanupDeadline := time.Now().Add(convergenceLimit)
	if cleanupDeadline.After(deadline) {
		cleanupDeadline = deadline
	}
	// A signal cancels execution, but permits cleanup within the original deadline.
	cleanup, stopCleanup := context.WithDeadline(context.WithoutCancel(ctx), cleanupDeadline)
	cleanupErr := r.cleanup(cleanup)
	stopCleanup()
	if err := errors.Join(executionErr, cleanupErr, ctx.Err()); err != nil {
		return err
	}
	result.Status, result.CleanupSucceeded, result.FinishedAt = passed, true, time.Now().UTC()
	if err := writeResult(input.output, result); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, os.Remove(input.output))
	}
	return nil
}

func main() {
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(signals, 35*time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil)).With("component", suiteName)
	input, err := parseOptions(os.Args[1:], sourceVersion)
	if errors.Is(err, flag.ErrHelp) {
		cancel()
		stop()
		return
	}
	if err == nil {
		err = run(ctx, input, invokeKubectl)
	}
	cancel()
	stop()
	if err != nil {
		logger.Error("conformance failed", "error", err)
		os.Exit(1)
	}
	logger.Info("conformance passed", "runId", input.runID, "result", input.output)
}
