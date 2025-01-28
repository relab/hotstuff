package queue_test

import (
	"testing"

	"github.com/relab/hotstuff/eventloop/queue"
)

func TestPopEmptyQueue(t *testing.T) {
	q := queue.New(1)
	elem, ok := q.Pop()
	if elem != nil || ok {
		t.Error("expected q.Pop() to return nil, false")
	}
}

func TestEmptyLen(t *testing.T) {
	q := queue.New(1)

	if q.Len() != 0 {
		t.Error("expected q.Len() to return 0")
	}
}

func TestPushAndPopWithCapacity1(t *testing.T) {
	q := queue.New(1)
	q.Push("hello")

	elem, ok := q.Pop()

	if elem.(string) != "hello" || !ok {
		t.Errorf("expected q.Pop() to return \"hello\", true")
	}
}

func TestPushAndThenLen(t *testing.T) {
	q := queue.New(1)
	q.Push("hello")

	if q.Len() != 1 {
		t.Errorf("expected q.Len() to return 1")
	}
}

func TestPushAndThenPopTwice(t *testing.T) {
	q := queue.New(1)
	q.Push("hello")

	elem, ok := q.Pop()
	if elem.(string) != "hello" || !ok {
		t.Errorf("expected q.Pop() to return \"hello\", true")
	}

	elem, ok = q.Pop()
	if elem != nil || ok {
		t.Error("expected q.Pop() to return nil, false")
	}
}

func TestPushWhenFull(t *testing.T) {
	q := queue.New(1)
	q.Push("hello")
	q.Push("world")

	elem, ok := q.Pop()
	if elem.(string) != "world" || !ok {
		t.Errorf("expected q.Pop() to return \"world\", true")
	}
}

func TestPushMultiple(t *testing.T) {
	q := queue.New(2)
	q.Push("hello")
	q.Push("world")

	elem, ok := q.Pop()
	if elem.(string) != "hello" || !ok {
		t.Errorf("expected q.Pop() to return \"hello\", true")
	}

	elem, ok = q.Pop()
	if elem.(string) != "world" || !ok {
		t.Errorf("expected q.Pop() to return \"world\", true")
	}
}

func TestLenWhenTailInFrontOfHead(t *testing.T) {
	q := queue.New(2)

	q.Push("hello")
	q.Push("world")
	q.Pop()
	q.Push("foo")

	if q.Len() != 2 {
		t.Error("expected q.Len() to return 2")
	}
}

func TestPopWhenTailInFrontOfHead(t *testing.T) {
	q := queue.New(2)

	q.Push("hello")
	q.Push("world")
	q.Pop()
	q.Push("test")

	elem, ok := q.Pop()
	if elem.(string) != "world" || !ok {
		t.Errorf("expected q.Pop() to return \"world\", true")
	}

}
