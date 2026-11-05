package fsnotify

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	releaseInterval        = 15 * time.Second
	maxReleaseInterval     = 2 * time.Minute
	pendingDeleteBatchSize = 100
)

type ReleaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
}

type watcher struct {
	cli  ReleaseIPsClient
	path string
	log  *zap.Logger

	pendingDelete chan string
	batchSize     int
	rescan        chan struct{}
	retryInterval time.Duration
	waitForRetry  func(context.Context, time.Duration) error
}

type releaseResult struct {
	retry             bool
	cnsReached        bool
	connectionFailure bool
}

// Create the AsyncDelete watcher.
func New(cli ReleaseIPsClient, path string, logger *zap.Logger) (*watcher, error) { //nolint
	// Add directory where intended deletes are kept
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, fs.ErrExist) { //nolint
		logger.Error("error making directory", zap.String("path", path), zap.Error(err))
		return nil, errors.Wrapf(err, "failed to create dir %s", path)
	}
	return &watcher{
		cli:           cli,
		path:          path,
		log:           logger,
		pendingDelete: make(chan string, pendingDeleteBatchSize),
		batchSize:     pendingDeleteBatchSize,
		rescan:        make(chan struct{}, 1),
		retryInterval: releaseInterval,
		waitForRetry:  waitForRetry,
	}, nil
}

// enqueuePendingDeletes streams the directory into the bounded channel. A full
// channel blocks this producer until the consumer frees capacity.
func (w *watcher) enqueuePendingDeletes(ctx context.Context) error {
	directory, err := os.Open(w.path)
	if err != nil {
		return errors.Wrap(err, "opening pending delete directory")
	}
	defer directory.Close()

	for {
		files, readErr := directory.ReadDir(w.batchSize)
		for _, file := range files {
			if !file.Type().IsRegular() {
				continue
			}
			select {
			case <-ctx.Done():
				return errors.Wrap(ctx.Err(), "enqueueing pending deletes")
			case w.pendingDelete <- file.Name():
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return errors.Wrap(readErr, "reading pending delete directory")
		}
	}
}

func (w *watcher) releasePendingDelete(ctx context.Context, containerID string) releaseResult {
	filePath := filepath.Join(w.path, containerID)
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return releaseResult{}
		}
		// Without the file we cannot know the pod interface ID, and a
		// release without it cannot match the assignment in CNS.
		w.log.Error("failed to open file", zap.String("containerID", containerID), zap.String("path", filePath), zap.Error(err))
		return releaseResult{retry: true}
	}

	data, errReadingFile := io.ReadAll(file)
	if errReadingFile != nil {
		w.log.Error("failed to read file content", zap.String("containerID", containerID), zap.String("path", filePath), zap.Error(errReadingFile))
		file.Close()
		return releaseResult{retry: true}
	}
	file.Close()
	if len(data) == 0 {
		w.log.Warn("pending delete file is empty", zap.String("containerID", containerID), zap.String("path", filePath))
		return releaseResult{retry: true}
	}
	podInterfaceID := string(data)

	w.log.Info("releasing IP for missed delete", zap.String("podInterfaceID", podInterfaceID), zap.String("containerID", containerID))
	if err := w.releaseIP(ctx, podInterfaceID, containerID); err != nil {
		w.log.Error("failed to release IP for missed delete", zap.String("containerID", containerID), zap.Error(err))
		var connectionErr *cnsclient.ConnectionFailureErr
		if errors.As(err, &connectionErr) {
			return releaseResult{retry: true, connectionFailure: true}
		}
		return releaseResult{retry: true, cnsReached: true}
	}
	w.log.Info("successfully released IP for missed delete", zap.String("containerID", containerID))
	if err := removeFile(containerID, w.path); err != nil {
		w.log.Error("failed to remove file for missed delete", zap.Error(err))
		return releaseResult{retry: true, cnsReached: true}
	}
	return releaseResult{cnsReached: true}
}

func (w *watcher) requestRescan() {
	select {
	case w.rescan <- struct{}{}:
	default:
	}
}

func waitForRetry(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// watchPendingDelete continuously consumes pending deletes.
func (w *watcher) watchPendingDelete(ctx context.Context) error {
	currentInterval := w.retryInterval
	var retryTimer *time.Timer
	var retryTimerC <-chan time.Time
	defer func() {
		if retryTimer != nil {
			retryTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "exiting watchPendingDelete")
		case <-retryTimerC:
			retryTimer = nil
			retryTimerC = nil
			w.requestRescan()
		case containerID := <-w.pendingDelete:
			result := w.releasePendingDelete(ctx, containerID)
			if result.cnsReached {
				currentInterval = w.retryInterval
			}
			if !result.retry {
				continue
			}
			if !result.connectionFailure {
				if retryTimer == nil {
					retryTimer = time.NewTimer(currentInterval)
					retryTimerC = retryTimer.C
				}
				continue
			}
			currentInterval *= 2
			if currentInterval > maxReleaseInterval {
				currentInterval = maxReleaseInterval
			}

			if retryTimer != nil {
				retryTimer.Stop()
				retryTimer = nil
				retryTimerC = nil
			}
			if err := w.waitForRetry(ctx, currentInterval); err != nil {
				return errors.Wrap(err, "waiting to retry pending delete")
			}
			w.requestRescan()
		}
	}
}

// watchFS produces pending deletes from coalesced filesystem events,
// consumer-requested retries, and watcher-error recovery. It does not poll.
func (w *watcher) watchFS(ctx context.Context) error {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "error creating fsnotify watcher")
	}
	defer fsWatcher.Close()

	if err := fsWatcher.Add(w.path); err != nil {
		w.log.Error("failed to add path to fsnotify watcher", zap.String("path", w.path), zap.Error(err))
		return errors.Wrap(err, "failed to add path to fsnotify watcher")
	}
	if err := w.enqueuePendingDeletes(ctx); err != nil {
		return err
	}

	w.log.Info("listening for events from fsnotify watcher")
	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "exiting watchFS")
		case <-w.rescan:
			if err := w.enqueuePendingDeletes(ctx); err != nil {
				return err
			}
		case event, ok := <-fsWatcher.Events:
			if !ok {
				return errors.New("fsnotify watcher closed")
			}
			if !event.Has(fsnotify.Write) {
				continue
			}
			w.log.Info("received filesystem event", zap.String("event", event.Name))
			w.requestRescan()
		case watcherErr := <-fsWatcher.Errors:
			w.log.Error("fsnotify watcher error", zap.Error(watcherErr))
			w.requestRescan()
		}
	}
}

// Start starts the filesystem watcher to handle async Pod deletes.
// Blocks until the context is closed; returns underlying fsnotify errors
// if something goes fatally wrong.
func (w *watcher) Start(ctx context.Context) error {
	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error { return w.watchPendingDelete(groupCtx) })
	g.Go(func() error { return w.watchFS(groupCtx) })
	return g.Wait() //nolint:wrapcheck // ignore
}

// AddFile creates new file using the containerID as name
func AddFile(podInterfaceID, containerID, path string) error {
	filePath := path + "/" + containerID
	f, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "error creating file")
	}
	_, writeErr := f.WriteString(podInterfaceID)
	if writeErr != nil {
		return errors.Wrap(writeErr, "error writing to file")
	}
	return errors.Wrap(f.Close(), "error adding file to directory")
}

// removeFile removes the file based on containerID
func removeFile(containerID, path string) error {
	filePath := path + "/" + containerID
	if err := os.Remove(filePath); err != nil {
		return errors.Wrap(err, "error deleting file")
	}
	return nil
}

// call cns ReleaseIPs
func (w *watcher) releaseIP(ctx context.Context, podInterfaceID, containerID string) error {
	ipconfigreq := &cns.IPConfigsRequest{
		PodInterfaceID:   podInterfaceID,
		InfraContainerID: containerID,
	}
	return errors.Wrap(w.cli.ReleaseIPs(ctx, *ipconfigreq), "failed to release IP from CNS")
}
