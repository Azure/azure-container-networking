package ipsets

type workerPool struct{}

func newWorkerPool(_ int) *workerPool {
	return &workerPool{}
}
