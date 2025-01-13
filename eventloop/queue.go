package eventloop

import "sync"

// ringBuffer is a thread-safe, bounded circular buffer.
// If an entry is pushed to the queue when it is full, the oldest entry will be dropped.
type ringBuffer struct {
	mut       sync.Mutex
	entries   []any
	head      int
	tail      int
	readyChan chan struct{}
}

func newRingBuffer(capacity uint) ringBuffer {
	if capacity == 0 {
		panic("capacity must be over 0")
	}

	return ringBuffer{
		entries:   make([]any, capacity),
		head:      -1,
		tail:      -1,
		readyChan: make(chan struct{}),
	}
}

// push adds an entry to the buffer in a FIFO fashion. If the queue is full, the first
// entry is dropped to make space for the newest entry.
func (q *ringBuffer) push(entry any) bool {
	q.mut.Lock()
	defer q.mut.Unlock()

	droppedOne := false

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
		droppedOne = true
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
	return droppedOne
}

// pop removes the first entry and returns it.
// If the buffer is empty, nil and false is returned.
func (q *ringBuffer) pop() (entry any, ok bool) {
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

// len returns the number of entries in the buffer.
func (q *ringBuffer) len() int {
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

// ready returns a channel that can block when the buffer
// contains at least one item.
func (q *ringBuffer) ready() <-chan struct{} {
	return q.readyChan
}
