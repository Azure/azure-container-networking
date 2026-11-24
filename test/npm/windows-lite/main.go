//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"
)

var errInput = errors.New("windows-lite: invalid runner input")

type options struct {
	kubeconfig   string
	daemonSet    string
	npmNamespace string
	image        string
	checkpoint   string
	snapshot     string
}

func parseOptions(args []string) (options, error) {
	var out options
	flags := flag.NewFlagSet(suiteName, flag.ContinueOnError)
	flags.StringVar(&out.kubeconfig, "kubeconfig", "", "existing private kubeconfig")
	flags.StringVar(&out.daemonSet, "daemonset", "", "explicit Windows NPM DaemonSet name")
	flags.StringVar(&out.npmNamespace, "npm-namespace", "kube-system", "Windows NPM namespace")
	flags.StringVar(&out.image, "npm-image", "", "complete immutable candidate child reference")
	flags.StringVar(&out.checkpoint, "checkpoint", "", "verify npm before or after externally run specs instead of running the cases")
	flags.StringVar(&out.snapshot, "snapshot", "", "private npm snapshot written before and verified after externally run specs")
	if err := flags.Parse(args); err != nil {
		return out, fmt.Errorf("parse flags: %w", err)
	}
	name := regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
	if flags.NArg() != 0 || !name.MatchString(out.daemonSet) || len(out.daemonSet) > 63 ||
		!name.MatchString(out.npmNamespace) || len(out.npmNamespace) > 63 ||
		out.kubeconfig == "" || out.image == "" {
		return out, fmt.Errorf("%w: kubeconfig, npm-image, and valid daemonset/namespace names are required", errInput)
	}
	if (out.checkpoint != "" && out.checkpoint != checkpointBefore && out.checkpoint != checkpointAfter) ||
		(out.checkpoint == "") != (out.snapshot == "") {
		return out, fmt.Errorf("%w: checkpoint must be before or after and requires a snapshot", errInput)
	}
	return out, nil
}

func run(ctx context.Context, input options, command commandFunc) error {
	kubeconfig, err := os.Lstat(input.kubeconfig)
	if err != nil {
		return fmt.Errorf("inspect kubeconfig: %w", err)
	}
	if !kubeconfig.Mode().IsRegular() || kubeconfig.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: kubeconfig must be a private regular file", errInput)
	}
	image, err := parseImage(input.image)
	if err != nil {
		return err
	}
	r := runner{
		kube: kubeClient{input.kubeconfig, command}, npmNamespace: input.npmNamespace,
		daemonSet: input.daemonSet, image: image,
	}
	switch input.checkpoint {
	case checkpointBefore:
		return r.recordCheckpoint(ctx, input.snapshot)
	case checkpointAfter:
		return r.verifyCheckpoint(ctx, input.snapshot)
	}
	cases, err := loadCorpus()
	if err != nil {
		return err
	}
	r.corpus = cases
	deadline, ok := ctx.Deadline()
	if !ok {
		return fmt.Errorf("%w: runner requires an overall deadline", errInput)
	}
	// Reserve the final convergence window for UID-bound namespace cleanup.
	work, cancel := context.WithDeadline(ctx, deadline.Add(-convergenceLimit))
	executionErr := r.execute(work)
	cancel()
	cleanupDeadline := time.Now().Add(convergenceLimit)
	if cleanupDeadline.After(deadline) {
		cleanupDeadline = deadline
	}
	// A signal cancels execution, but permits cleanup within the original deadline.
	cleanup, stopCleanup := context.WithDeadline(context.WithoutCancel(ctx), cleanupDeadline)
	cleanupErr := r.cleanup(cleanup)
	stopCleanup()
	return errors.Join(executionErr, cleanupErr, ctx.Err())
}

func main() {
	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(signals, 35*time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil)).With("component", suiteName)
	input, err := parseOptions(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		cancel()
		stop()
		return
	}
	if err == nil {
		if input.checkpoint != "" {
			logger = logger.With("checkpoint", input.checkpoint)
		}
		err = run(ctx, input, invokeKubectl)
	}
	cancel()
	stop()
	if err != nil {
		logger.Error("conformance failed", "error", err)
		os.Exit(1)
	}
	if input.checkpoint != "" {
		logger.Info("npm checkpoint passed", "image", input.image)
		return
	}
	logger.Info("conformance passed", "image", input.image, "cases", len(requiredCases))
}
