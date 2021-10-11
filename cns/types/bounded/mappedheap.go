package bounded

import (
	"container/heap"
)

// Item describes a type accepted by the mapped heap implementation.
type Item interface {
	// Key is used for map index operations.
	Key() string
	// Less is used for heap sorting operations.
	Less(Item) bool
	// SetIndex is called by heap implementations to set the Item heap index.
	SetIndex(int)
	// Index returns the index of this Item.
	Index() int
}

var _ heap.Interface = (*MappedHeap)(nil)

// MappedHeap is a combination of map and heap structures which allows for
// efficient sorting, uniqueness guarantees, and constant time lookups.
// Implements heap.Interface.
type MappedHeap struct {
	m     map[string]Item
	items []Item
}

func NewMappedHeap() *MappedHeap {
	return &MappedHeap{
		m: make(map[string]Item),
	}
}

func (mh MappedHeap) Contains(key string) (int, bool) {
	item, ok := mh.m[key]
	if ok {
		return item.Index(), true
	}
	return -1, false
}

func (mh MappedHeap) Len() int {
	return len(mh.items)
}

func (mh MappedHeap) Less(i, j int) bool {
	return mh.items[i].Less(mh.items[j])
}

func (mh *MappedHeap) Swap(i, j int) {
	mh.items[i], mh.items[j] = mh.items[j], mh.items[i]
	mh.items[i].SetIndex(i)
	mh.items[j].SetIndex(j)
}

func (mh *MappedHeap) Push(x interface{}) {
	n := len(mh.items)
	item := x.(Item)
	item.SetIndex(n)
	mh.items = append(mh.items, item)
	mh.m[item.Key()] = item
}

func (mh *MappedHeap) Pop() interface{} {
	old := mh.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.SetIndex(-1)
	mh.items = old[0 : n-1]
	delete(mh.m, item.Key())
	return item
}
