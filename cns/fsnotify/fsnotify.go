package fsnotify

import (
	"context"
	"io"
	"io/fs"
	"os"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const pendingDeleteBatchSize = 100

type ReleaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
}

type watcher struct {
	cli  ReleaseIPsClient
	path string
	log  *zap.Logger

	pendingDelete chan string
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
	}, nil
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
	for {
		var containerID string
		select {
		case <-ctx.Done():
			return
		case containerID = <-w.pendingDelete:
		}
		// read file contents
		filePath := w.path + "/" + containerID
		file, err := os.Open(filePath)
		if err != nil {
			// Without the file we cannot know the pod interface ID, and a
			// release without it cannot match the assignment in CNS.
			w.log.Error("failed to open file", zap.String("containerID", containerID), zap.String("path", filePath), zap.Error(err))
			continue
		}

		data, errReadingFile := io.ReadAll(file)
		if errReadingFile != nil {
			w.log.Error("failed to read file content", zap.String("containerID", containerID), zap.String("path", filePath), zap.Error(errReadingFile))
			file.Close()
			continue
		}
		file.Close()
		podInterfaceID := string(data)

		w.log.Info("releasing IP for missed delete", zap.String("podInterfaceID", podInterfaceID), zap.String("containerID", containerID))
		if err := w.releaseIP(ctx, podInterfaceID, containerID); err != nil {
			w.log.Error("failed to release IP for missed delete", zap.String("containerID", containerID), zap.Error(err))
			continue
		}
		w.log.Info("successfully released IP for missed delete", zap.String("containerID", containerID))
		if err := removeFile(containerID, w.path); err != nil {
			w.log.Error("failed to remove file for missed delete", zap.Error(err))
		}
	}
}

// watchPendingDelete processes pending release IPs as they are discovered.
func (w *watcher) watchPendingDelete(ctx context.Context) error {
	w.releaseAll(ctx)
	return errors.Wrap(ctx.Err(), "exiting watchPendingDelete")
}

func (w *watcher) readPendingDeletes(ctx context.Context) error {
	directory, err := os.Open(w.path)
	if err != nil {
		return errors.Wrap(err, "opening pending delete directory")
	}
	defer directory.Close()

	for {
		files, readErr := directory.Readdir(pendingDeleteBatchSize)
		for _, file := range files {
			select {
			case <-ctx.Done():
				return errors.Wrap(ctx.Err(), "reading pending deletes")
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

	// Start watching the directory, so that we don't miss any events.
	err = watcher.Add(w.path)
	if err != nil {
		w.log.Error("failed to add path to fsnotify watcher", zap.String("path", w.path), zap.Error(err))
		return errors.Wrap(err, "failed to add path to fsnotify watcher")
	}
	// List the directory and create synthetic events for any existing items.
	w.log.Info("listing directory", zap.String("path", w.path))
	if err := w.readPendingDeletes(ctx); err != nil {
		return err
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
			if err := w.readPendingDeletes(ctx); err != nil {
				return err
			}
		case watcherErr := <-watcher.Errors:
			w.log.Error("fsnotify watcher error", zap.Error(watcherErr))
		}
	}
}

// Start starts the filesystem watcher to handle async Pod deletes.
// Blocks until the context is closed; returns underlying fsnotify errors
// if something goes fatally wrong.
func (w *watcher) Start(ctx context.Context) error {
	g, groupCtx := errgroup.WithContext(ctx)
	// Start watching for enqueued missed deletes so that we process them as soon as they arrive.
	g.Go(func() error { return w.watchPendingDelete(groupCtx) })
	// Start watching for changes to the filesystem so that we don't miss any async deletes.
	g.Go(func() error { return w.watchFS(groupCtx) })
	// the first error from the errgroup will trigger context cancellation for other goroutines in the group.
	// this will block until all goroutines complete and return the first error.
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
