package eventloop

import "sync"

// queue is a bounded circular buffer.
// If an entry is pushed to the queue when it is full, the oldest entry will be dropped.
type queue struct {
	mut       sync.Mutex
	entries   []interface{}
	head      int
	tail      int
	readyChan chan struct{}
}

func newQueue(capacity uint) queue {
	return queue{
		entries:   make([]interface{}, capacity),
		head:      -1,
		tail:      -1,
		readyChan: make(chan struct{}),
	}
}

func (q *queue) push(entry interface{}) {
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

func (q *queue) pop() (entry interface{}, ok bool) {
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

func (q *queue) len() int {
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

func (q *queue) ready() <-chan struct{} {
	return q.readyChan
}
