package queue

import "sync"

// Queue is a bounded circular buffer.
// If an entry is pushed to the Queue when it is full, the oldest entry will be dropped.
type Queue struct {
	mut       sync.Mutex
	entries   []any
	head      int
	tail      int
	readyChan chan struct{}
}

func NewQueue(capacity uint) Queue {
	return Queue{
		entries:   make([]any, capacity),
		head:      -1,
		tail:      -1,
		readyChan: make(chan struct{}),
	}
}

func (q *Queue) Push(entry any) {
	q.mut.Lock()
	defer q.mut.Unlock()

	if len(q.entries) == 0 {
		panic("cannot push to a queue with capacity 0")
	}

	pos := q.tail + 1
	if pos == len(q.entries) {
		pos = 0
	}
	if pos == q.head {
		// drop the entry at the head of the queue
		q.head++
		if q.head == len(q.entries) {
			q.head = 0
		}
	}
	q.entries[pos] = entry
	q.tail = pos

	if q.head == -1 {
		q.head = pos
	}

	select {
	case q.readyChan <- struct{}{}:
	default:
	}
}

func (q *Queue) Pop() (entry any, ok bool) {
	q.mut.Lock()
	defer q.mut.Unlock()

	if q.head == -1 {
		return nil, false
	}

	entry = q.entries[q.head]

	if q.head == q.tail {
		q.head = -1
		q.tail = -1
	} else {
		q.head++
		if q.head == len(q.entries) {
			q.head = 0
		}
	}

	return entry, true
}

func (q *Queue) Len() int {
	q.mut.Lock()
	defer q.mut.Unlock()

	if q.head == -1 {
		return 0
	}

	if q.head <= q.tail {
		return q.tail - q.head + 1
	}

	return len(q.entries) - q.head + q.tail + 1
}

func (q *Queue) Ready() <-chan struct{} {
	return q.readyChan
}
