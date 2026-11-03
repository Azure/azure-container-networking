package fsnotify

import (
	"context"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	cnsclient "github.com/Azure/azure-container-networking/cns/client"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	releaseInterval     = 15 * time.Second
	maxReleaseInterval  = 2 * time.Minute
	defaultReleaseBatch = 100
)

type ReleaseIPsClient interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
}

type watcher struct {
	cli       ReleaseIPsClient
	path      string
	log       *zap.Logger
	directory *os.File
	batchSize int
}

// Create the AsyncDelete watcher.
func New(cli ReleaseIPsClient, path string, logger *zap.Logger) (*watcher, error) { //nolint
	// Add directory where intended deletes are kept
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, fs.ErrExist) { //nolint
		logger.Error("error making directory", zap.String("path", path), zap.Error(err))
		return nil, errors.Wrapf(err, "failed to create dir %s", path)
	}
	return &watcher{
		cli:       cli,
		path:      path,
		log:       logger,
		batchSize: defaultReleaseBatch,
	}, nil
}

func (w *watcher) closeDirectory() {
	if w.directory == nil {
		return
	}
	if err := w.directory.Close(); err != nil {
		w.log.Warn("closing pending delete directory failed", zap.String("path", w.path), zap.Error(err))
	}
	w.directory = nil
}

func (w *watcher) nextBatch() ([]fs.FileInfo, error) {
	if w.directory == nil {
		directory, err := os.Open(w.path)
		if err != nil {
			return nil, errors.Wrap(err, "opening pending delete directory")
		}
		w.directory = directory
	}

	files, err := w.directory.Readdir(w.batchSize)
	if errors.Is(err, io.EOF) {
		w.closeDirectory()
		return files, nil
	}
	return files, errors.Wrap(err, "reading pending delete directory")
}

// releaseBatch processes a bounded number of pending deletes. It returns true
// when CNS is unreachable so the caller can back off before retrying.
func (w *watcher) releaseBatch(ctx context.Context) (bool, error) {
	files, err := w.nextBatch()
	if err != nil {
		return false, err
	}

	for _, fileInfo := range files {
		if !fileInfo.Mode().IsRegular() {
			continue
		}
		containerID := fileInfo.Name()
		// read file contents
		filePath := w.path + string(os.PathSeparator) + containerID
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
			var connectionErr *cnsclient.ConnectionFailureErr
			if errors.As(err, &connectionErr) {
				w.closeDirectory()
				return true, nil
			}
			continue
		}
		w.log.Info("successfully released IP for missed delete", zap.String("containerID", containerID))
		if err := removeFile(containerID, w.path); err != nil {
			w.log.Error("failed to remove file for missed delete", zap.Error(err))
		}
	}
	return false, nil
}

func nextReleaseInterval(current time.Duration, connectionFailure bool) time.Duration {
	if !connectionFailure {
		return releaseInterval
	}
	current *= 2
	if current > maxReleaseInterval {
		return maxReleaseInterval
	}
	return current
}

// watchPendingDelete periodically processes a bounded batch of pending releases.
func (w *watcher) watchPendingDelete(ctx context.Context) error {
	timer := time.NewTimer(releaseInterval)
	defer timer.Stop()
	currentInterval := releaseInterval

	for {
		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "exiting watchPendingDelete")
		case <-timer.C:
			connectionFailure, err := w.releaseBatch(ctx)
			if err != nil {
				return err
			}
			currentInterval = nextReleaseInterval(currentInterval, connectionFailure)
			timer.Reset(currentInterval)
		}
	}
}

// Start starts the filesystem watcher to handle async Pod deletes.
// Blocks until the context is closed or the queue cannot be read.
func (w *watcher) Start(ctx context.Context) error {
	defer w.closeDirectory()
	return w.watchPendingDelete(ctx)
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
