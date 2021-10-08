package bounded

import (
	"container/heap"
	"sync"
	"time"
)

var _ Item = (*TimedItem)(nil)

// TimedItem implements Item for a string: time.Time tuple.
type TimedItem struct {
	Name  string
	Time  time.Time
	index int
}

func (t *TimedItem) Key() string {
	return t.Name
}

func (t *TimedItem) Less(o Item) bool {
	other := o.(*TimedItem)
	return t.Time.Before(other.Time)
}

func (t *TimedItem) Index() int {
	return t.index
}

func (t *TimedItem) SetIndex(i int) {
	t.index = i
}

type TimedSet struct {
	sync.Mutex
	capacity int
	items    *MappedHeap
}

func NewTimedSet(c int) *TimedSet {
	return &TimedSet{
		capacity: c,
		items:    NewMappedHeap(),
	}
}

// Push registers the passed key and saves the timestamp it is first registered.
// If the key is already registered, does not overwrite the saved timestamp.
func (ts *TimedSet) Push(key string) {
	ts.Lock()
	defer ts.Unlock()
	if _, ok := ts.items.Contains(key); ok {
		return
	}
	if ts.items.Len() >= ts.capacity {
		_ = heap.Pop(ts.items)
	}
	item := &TimedItem{Name: key}
	item.Time = time.Now()
	heap.Push(ts.items, item)
}

// Pop returns the elapsed duration since the passed key was first registered,
// or -1 if it is not found.
func (ts *TimedSet) Pop(key string) time.Duration {
	ts.Lock()
	defer ts.Unlock()
	idx, ok := ts.items.Contains(key)
	if !ok {
		return -1
	}
	item := heap.Remove(ts.items, idx)
	return time.Since(item.(*TimedItem).Time)
}
