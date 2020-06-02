package data

import (
	"testing"
)

func TestCmdSetAdd(t *testing.T) {
	s := NewCommandSet()
	c1 := Command("Hello")
	c2 := Command("World")

	s.Add(c1, c2)

	if c1 != s.order.Front().Value.(*cmdElement).cmd {
		t.Fatalf("c1 is not at the front of the list!")
	}

	if c2 != s.order.Back().Value.(*cmdElement).cmd {
		t.Fatalf("c2 is not at the back of the list")
	}
}

func TestCmdSetGet(t *testing.T) {
	s := NewCommandSet()
	c1 := Command("Hello")
	c2 := Command("World")

	s.Add(c1, c2)

	cs := s.GetFirst(1)
	if len(cs) < 1 || c1 != cs[0] {
		t.Fatalf("c1 was not returned!")
	}
}

func TestCmdSetAvoidsDuplicates(t *testing.T) {
	s := NewCommandSet()
	c1 := Command("Hello")
	c2 := Command("World")
	c3 := c1

	s.Add(c1, c2, c3)

	if c1 != s.order.Front().Value.(*cmdElement).cmd {
		t.Fatalf("c1 is not at the front of the list!")
	}

	if c2 != s.order.Back().Value.(*cmdElement).cmd {
		t.Fatalf("c2 is not at the back of the list")
	}
}

func TestCmdSetRemove(t *testing.T) {
	s := NewCommandSet()
	c1 := Command("Hello")
	c2 := Command("World")
	s.Add(c1, c2)

	s.Remove(c1)

	cs := s.GetFirst(1)
	if len(cs) < 1 || c2 != cs[0] {
		t.Fatalf("c2 is not at the front of the list")
	}
}

func TestCmdSetMarkProposed(t *testing.T) {
	s := NewCommandSet()
	c1 := Command("Hello")
	c2 := Command("World")
	s.Add(c1, c2)

	s.MarkProposed(c1)

	cs := s.GetFirst(1)
	if len(cs) < 1 || c2 != cs[0] {
		t.Fatalf("c2 is not at the front of the list")
	}
}

func TestCmdSetTrimToLen(t *testing.T) {
	s := NewCommandSet()
	c1 := Command("Hello")
	c2 := Command("World")
	s.Add(c1, c2)
	s.MarkProposed(c1)

	s.TrimToLen(1)

	cs := s.GetFirst(1)
	if len(cs) < 1 || c2 != cs[0] {
		t.Fatalf("c2 is not at the front of the list")
	}
}
