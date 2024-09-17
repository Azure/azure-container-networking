package fsnotify

import (
	"context"
	"io"
	"io/fs"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/Azure/azure-container-networking/cns"
	"github.com/Azure/azure-container-networking/cns/configuration"
	"github.com/Azure/azure-container-networking/cns/hnsclient"
	"github.com/Azure/azure-container-networking/cns/logger"
	"github.com/Azure/azure-container-networking/cns/restserver"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

const DefaultNetworkID = "azure"

type endpointManager interface {
	ReleaseIPs(ctx context.Context, ipconfig cns.IPConfigsRequest) error
	GetEndpoint(ctx context.Context, endpointID string) (*restserver.GetEndpointResponse, error)
}

type watcher struct {
	cli           endpointManager
	path          string
	pendingDelete map[string]struct{}
	lock          sync.Mutex
	cnsconfig     *configuration.CNSConfig
}

// Create the AsyncDelete watcher.
func New(cnsconfig *configuration.CNSConfig, cli endpointManager, path string) (*watcher, error) { //nolint
	// Add directory where intended deletes are kept
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, fs.ErrExist) { //nolint
		logger.Errorf("error making directory %s , %s", path, err.Error())
		return nil, errors.Wrapf(err, "failed to create dir %s", path)
	}
	return &watcher{
		cli:           cli,
		path:          path,
		pendingDelete: make(map[string]struct{}),
		cnsconfig:     cnsconfig,
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
	w.lock.Lock()
	defer w.lock.Unlock()
	for containerID := range w.pendingDelete {
		logger.Printf("deleting Endpoint asynchronously")
		// read file contents
		filepath := w.path + "/" + containerID
		file, err := os.Open(filepath)
		if err != nil {
			logger.Errorf("failed to open file %s", err.Error())
		}

		data, errReadingFile := io.ReadAll(file)
		if errReadingFile != nil {
			logger.Errorf("failed to read file content %s", errReadingFile)
		}
		file.Close()
		podInterfaceID := string(data)
		// in case of stateless CNI for Windows, CNS needs to remove HNS endpoitns first
		if isStalessCNIMode(w.cnsconfig) {
			logger.Printf("deleting HNS Endpoint asynchronously")
			// remove HNS endpoint
			if err := w.deleteEndpoint(ctx, containerID); err != nil {
				logger.Errorf("failed to remove HNS endpoint %s", err.Error())
				continue
			}
		}
		logger.Printf("releasing IP for missed delete: podInterfaceID :%s containerID:%s", podInterfaceID, containerID)
		if err := w.releaseIP(ctx, podInterfaceID, containerID); err != nil {
			logger.Errorf("failed to release IP for missed delete: podInterfaceID :%s containerID:%s", podInterfaceID, containerID)
			continue
		}
		logger.Printf("successfully released IP for missed delete: podInterfaceID :%s containerID:%s", podInterfaceID, containerID)
		delete(w.pendingDelete, containerID)
		if err := removeFile(containerID, w.path); err != nil {
			logger.Errorf("failed to remove file for missed delete %s", err.Error())
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
			logger.Printf("processing pending missed deletes, count: %v", n)
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

	// Start watching the directory, so that we don't miss any events.
	err = watcher.Add(w.path)
	if err != nil {
		logger.Errorf("failed to add path %s to fsnotify watcher %s", w.path, err.Error())
		return errors.Wrap(err, "failed to add path to fsnotify watcher")
	}
	// List the directory and creates synthetic events for any existing items.
	logger.Printf("listing directory:%s", w.path)
	dirContents, err := os.ReadDir(w.path)
	if err != nil {
		logger.Errorf("error reading deleteID directory %s, %s", w.path, err.Error())
		return errors.Wrapf(err, "failed to read %s", w.path)
	}
	if len(dirContents) == 0 {
		logger.Printf("no missed deletes found")
	}
	w.lock.Lock()
	for _, file := range dirContents {
		logger.Printf("adding missed delete from file %s", file.Name())
		w.pendingDelete[file.Name()] = struct{}{}
	}
	w.lock.Unlock()

	// Start listening for events.
	logger.Printf("listening for events from fsnotify watcher")
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
			logger.Printf("received create event %s", event.Name)
			w.lock.Lock()
			w.pendingDelete[event.Name] = struct{}{}
			w.lock.Unlock()
		case watcherErr := <-watcher.Errors:
			logger.Errorf("fsnotify watcher error %s", watcherErr.Error())
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

// call GetEndpoint API to get the state and then remove assiciated HNS
func (w *watcher) deleteEndpoint(ctx context.Context, containerid string) error {
	endpointResponse, err := w.cli.GetEndpoint(ctx, containerid)
	if err != nil {
		return errors.Wrap(err, "failed to read the endpoint from CNS state")
	}
	for _, ipInfo := range endpointResponse.EndpointInfo.IfnameToIPMap {
		hnsEndpointID := ipInfo.HnsEndpointID
		// we need to get the HNSENdpoint via the IP address if the HNSEndpointID is not present in the statefile
		if ipInfo.HnsEndpointID == "" {
			// TODO: the HSN client for windows needs to be refactored:
			// remove hnsclient_linux.go and hnsclient_windows.go and instead have endpoint_linux.go and endpoint_windows.go
			// and abstract hns changes in endpoint_windows.go
			if hnsEndpointID, err = hnsclient.GetHNSEndpointbyIP(ipInfo.IPv4, ipInfo.IPv6, DefaultNetworkID); err != nil {
				return errors.Wrap(err, "failed to find HNS endpoint with id")
			}
		}
		logger.Printf("deleting HNS Endpoint with id %v", hnsEndpointID)
		if err := hnsclient.DeleteHNSEndpointbyID(hnsEndpointID); err != nil {
			return errors.Wrap(err, "failed to delete HNS endpoint with id "+ipInfo.HnsEndpointID)
		}
	}
	return nil
}

// isStalessCNIMode verify if the CNI is running stateless mode
func isStalessCNIMode(cnsconfig *configuration.CNSConfig) bool {
	if !cnsconfig.InitializeFromCNI && cnsconfig.ManageEndpointState && runtime.GOOS == "windows" {
		return true
	}
	return false

}
