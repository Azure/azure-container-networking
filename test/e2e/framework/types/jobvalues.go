package types

import "sync"

type JobValues struct {
	RWLock sync.RWMutex
	kv     map[string]string
}

func (j *JobValues) New() *JobValues {
	return &JobValues{
		kv: make(map[string]string),
	}
}

func (j *JobValues) Contains(key string) bool {
	j.RWLock.RLock()
	defer j.RWLock.RUnlock()
	_, ok := j.kv[key]
	return ok
}

func (j *JobValues) Get(key string) string {
	j.RWLock.RLock()
	defer j.RWLock.RUnlock()
	return j.kv[key]
}

func (j *JobValues) Set(key, value string) {
	j.RWLock.Lock()
	defer j.RWLock.Unlock()
	j.kv[key] = value
}
