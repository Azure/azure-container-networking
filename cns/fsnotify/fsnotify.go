package fsnotify

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type releaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
}

type watcher struct {
	cli  releaseIPsClient
	path string
	log  *zap.Logger

	pendingDelete map[string]struct{}
	lock          sync.Mutex
}

// Create the AsyncDelete watcher.
func New(cli releaseIPsClient, path string, logger *zap.Logger) *watcher { //nolint
	// Add directory where intended deletes are kept
	if err := os.Mkdir(path, 0o755); err != nil { //nolint
		logger.Error("error making directory", zap.String("path", path), zap.Error(err))
	}
	return &watcher{
		cli:           cli,
		path:          path,
		log:           logger,
		pendingDelete: make(map[string]struct{}),
	}
}

// releaseAll locks and iterates the pendingDeletes map and calls CNS to
// release the IP for any Pod containerIDs present. When an IP is released
// that entry is removed from the map and the file is deleted. If the file
// fails to delete, we still remove it from the map so that we don't retry
// it during the life of this process, but we may retry it on a subsequent
// invocation of CNS. This is okay because calling releaseIP on an already
// processed containerID is a no-op, and we may be able to delete the file
// during that future retry.
func (w *watcher) releaseAll(ctx context.Context) {
	w.lock.Lock()
	defer w.lock.Unlock()
	for containerID := range w.pendingDelete {
		// read file contents
		filepath := w.path + "/" + containerID
		file, err := os.Open(filepath)
		if err != nil {
			w.log.Error("failed to open file", zap.Error(err))
		}

		data, errReadingFile := io.ReadAll(file)
		if errReadingFile != nil {
			w.log.Error("failed to read file content", zap.Error(errReadingFile))
		}
		file.Close()
		podInterfaceID := string(data)

		w.log.Info("releasing IP for missed delete", zap.String("podInterfaceID", podInterfaceID), zap.String("containerID", containerID))
		if err := w.releaseIP(ctx, podInterfaceID, containerID); err != nil {
			w.log.Error("failed to release IP for missed delete", zap.String("containerID", containerID), zap.Error(err))
			continue
		}
		w.log.Info("successfully released IP for missed delete", zap.String("containerID", containerID))
		delete(w.pendingDelete, containerID)
		if err := removeFile(containerID, w.path); err != nil {
			w.log.Error("failed to remove file for missed delete", zap.Error(err))
		}
	}
}

// watchPendingDelete periodically checks the map for pending release IPs
// and calls releaseAll to process the contents when present.
func (w *watcher) watchPendingDelete(ctx context.Context) error {
	ticker := time.NewTicker(15 * time.Second) //nolint
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "exiting watchPendingDelete")
		case <-ticker.C:
			n := len(w.pendingDelete)
			if n == 0 {
				continue
			}
			w.log.Info("processing pending missed deletes", zap.Int("count", n))
			w.releaseAll(ctx)
		}
	}
}

// watchFS starts the fsnotify watcher and handles events for file creation
// or deletion in the missed pending delete directory. A file creation event
// indicates that CNS missed the delete call for a containerID and needs
// to process the release IP asynchronously.
func (w *watcher) watchFS(ctx context.Context) error {
	// Create new fs watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return errors.Wrap(err, "error creating fsnotify watcher")
	}
	defer watcher.Close()

	err = watcher.Add(w.path)
	if err != nil {
		w.log.Error("failed to add path to fsnotify watcher", zap.String("path", w.path), zap.Error(err))
	}
	// Start listening for events.
	w.log.Info("listening for events from fsnotify watcher")
	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "exiting watchFS")
		case event, ok := <-watcher.Events:
			if !ok {
				return errors.New("fsnotify watcher closed")
			}
			if !event.Has(fsnotify.Create) {
				// discard any event that is not a file Create
				continue
			}
			w.log.Info("received create event", zap.String("event", event.Name))
			w.lock.Lock()
			w.pendingDelete[event.Name] = struct{}{}
			w.lock.Unlock()
		case watcherErr := <-watcher.Errors:
			w.log.Error("fsnotify watcher error", zap.Error(watcherErr))
		}
	}
}

// readFS lists the directory and enqueues any missed deletes that are already
// present on-disk.
func (w *watcher) readFS() error {
	w.log.Info("listing directory", zap.String("path", w.path))
	dirContents, err := os.ReadDir(w.path)
	if err != nil {
		w.log.Error("error reading deleteID directory", zap.String("path", w.path), zap.Error(err))
		return errors.Wrapf(err, "failed to read %s", w.path)
	}
	if len(dirContents) == 0 {
		w.log.Info("no missed deletes found")
		return nil
	}
	w.lock.Lock()
	for _, file := range dirContents {
		w.log.Info("adding missed delete from file", zap.String("name", file.Name()))
		w.pendingDelete[file.Name()] = struct{}{}
	}
	w.lock.Unlock()
	return nil
}

// WatchFS starts the filesystem watcher to handle async Pod deletes.
// Blocks until the context is closed; returns underlying fsnotify errors
// if something goes fatally wrong.
func (w *watcher) Start(ctx context.Context) error {
	errs := make(chan error)
	// Start watching for enqueued missed deletes so that we process them as soon as they arrive.
	go func(errs chan<- error) {
		errs <- w.watchPendingDelete(ctx)
	}(errs)

	// Start watching for changes to the filesystem so that we don't miss any async deletes.
	go func(errs chan<- error) {
		errs <- w.watchFS(ctx)
	}(errs)

	// Read the directory to enqueue any missed deletes that are already present on-disk.
	if err := w.readFS(); err != nil {
		return err
	}

	// block until one of the goroutines returns an error
	err := <-errs
	return err
}

// AddFile creates new file using the containerID as name
func AddFile(podInterfaceID, containerID, path string) error {
	filepath := path + "/" + containerID
	f, err := os.Create(filepath)
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
	filepath := path + "/" + containerID
	if err := os.Remove(filepath); err != nil {
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
