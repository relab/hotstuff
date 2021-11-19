package eventloop

import "testing"

func TestPopEmptyQueue(t *testing.T) {
	q := newQueue(1)
	elem, ok := q.pop()
	if elem != nil || ok {
		t.Error("expected q.pop() to return nil, false")
	}
}

func TestEmptyLen(t *testing.T) {
	q := newQueue(1)

	if q.len() != 0 {
		t.Error("expected q.len() to return 0")
	}
}

func TestPushAndPopWithCapacity1(t *testing.T) {
	q := newQueue(1)
	q.push("hello")

	elem, ok := q.pop()

	if elem.(string) != "hello" || !ok {
		t.Errorf("expected q.pop() to return \"hello\", true")
	}
}

func TestPushAndThenLen(t *testing.T) {
	q := newQueue(1)
	q.push("hello")

	if q.len() != 1 {
		t.Errorf("expected q.len() to return 1")
	}
}

func TestPushAndThenPopTwice(t *testing.T) {
	q := newQueue(1)
	q.push("hello")

	elem, ok := q.pop()
	if elem.(string) != "hello" || !ok {
		t.Errorf("expected q.pop() to return \"hello\", true")
	}

	elem, ok = q.pop()
	if elem != nil || ok {
		t.Error("expected q.pop() to return nil, false")
	}
}

func TestPushWhenFull(t *testing.T) {
	q := newQueue(1)
	q.push("hello")
	q.push("world")

	elem, ok := q.pop()
	if elem.(string) != "world" || !ok {
		t.Errorf("expected q.pop() to return \"world\", true")
	}
}

func TestPushMultiple(t *testing.T) {
	q := newQueue(2)
	q.push("hello")
	q.push("world")

	elem, ok := q.pop()
	if elem.(string) != "hello" || !ok {
		t.Errorf("expected q.pop() to return \"hello\", true")
	}

	elem, ok = q.pop()
	if elem.(string) != "world" || !ok {
		t.Errorf("expected q.pop() to return \"world\", true")
	}
}

func TestLenWhenTailInFrontOfHead(t *testing.T) {
	q := newQueue(2)

	q.push("hello")
	q.push("world")
	q.pop()
	q.push("foo")

	if q.len() != 2 {
		t.Error("expected q.len() to return 2")
	}
}

func TestPopWhenTailInFrontOfHead(t *testing.T) {
	q := newQueue(2)

	q.push("hello")
	q.push("world")
	q.pop()
	q.push("test")

	elem, ok := q.pop()
	if elem.(string) != "world" || !ok {
		t.Errorf("expected q.pop() to return \"world\", true")
	}

}
