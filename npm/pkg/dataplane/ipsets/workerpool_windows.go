package ipsets

import (
	"sync"

	"github.com/Microsoft/hcsshim/hcn"
)

type thread struct {
	id     int
	op     hcn.RequestType
	wg     *sync.WaitGroup
	ipsets map[string]*hcn.SetPolicySetting
}

type workerPool struct {
	sync.Mutex
	semaphore chan struct{}
	threads   map[int]*thread
}

func newWorkerPool(maxThreads int) *workerPool {
	return &workerPool{
		semaphore: make(chan struct{}, maxThreads),
		threads:   make(map[int]*thread),
	}
}
