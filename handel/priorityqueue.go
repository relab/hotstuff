package handel

import (
	"container/heap"
	"sync"
)

// An heapItem is something we manage in a priority queue.
type heapItem struct {
	contribution contribution // The value of the item; arbitrary.
	score        int          // The priority of the item in the queue.
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

type verificationQueue struct {
	mut   sync.Mutex
	inner verificationQueueInner
	ready chan struct{}
}

func newVerificationQueue() (q verificationQueue) {
	heap.Init(&q.inner)
	q.ready = make(chan struct{})
	return
}

func (q *verificationQueue) push(score int, contribution contribution) {
	q.mut.Lock()
	defer q.mut.Unlock()
	heap.Push(&q.inner, heapItem{
		contribution: contribution,
		score:        score,
	})
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *verificationQueue) pop() (_ contribution, ok bool) {
	q.mut.Lock()
	defer q.mut.Unlock()
	item := heap.Pop(&q.inner)
	if item == nil {
		return contribution{}, false
	}
	return item.(heapItem).contribution, true
}

func (q *verificationQueue) wait() {
	<-q.ready
}

// A verificationQueueInner implements heap.Interface and holds Items.
type verificationQueueInner []heapItem

func (pq verificationQueueInner) Len() int { return len(pq) }

func (pq verificationQueueInner) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].score > pq[j].score
}

func (pq verificationQueueInner) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *verificationQueueInner) Push(x interface{}) {
	n := len(*pq)
	item := x.(heapItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *verificationQueueInner) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = heapItem{} // avoid memory leak
	// item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// update modifies the priority and value of an Item in the queue.
// func (pq *verificationQueue) update(item *heapItem, value contribution, priority int) {
// 	item.contribution = value
// 	item.score = priority
// 	heap.Fix(pq, item.index)
// }
