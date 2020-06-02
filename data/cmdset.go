package data

import (
	"container/list"
	"sync"
)

type cmdElement struct {
	cmd      Command
	proposed bool
}

// CommandSet is a linkedhashset for Commands
type CommandSet struct {
	mut   sync.Mutex
	set   map[Command]*list.Element
	order list.List
}

func NewCommandSet() *CommandSet {
	c := &CommandSet{
		set: make(map[Command]*list.Element),
	}
	c.order.Init()
	return c
}

// Add adds cmds to the set. Duplicate entries are ignored
func (s *CommandSet) Add(cmds ...Command) {
	s.mut.Lock()
	defer s.mut.Unlock()

	for _, cmd := range cmds {
		// avoid duplicates
		if _, ok := s.set[cmd]; ok {
			continue
		}
		e := s.order.PushBack(&cmdElement{cmd: cmd})
		s.set[cmd] = e
	}
}

// Remove removes cmds from the set
func (s *CommandSet) Remove(cmds ...Command) {
	s.mut.Lock()
	defer s.mut.Unlock()

	for _, cmd := range cmds {
		if e, ok := s.set[cmd]; ok {
			// remove e from list and set
			delete(s.set, cmd)
			s.order.Remove(e)
		}
	}
}

// GetFirst returns the n first non-proposed commands in the set
func (s *CommandSet) GetFirst(n int) []Command {
	s.mut.Lock()
	defer s.mut.Unlock()

	if len(s.set) == 0 {
		return nil
	}

	cmds := make([]Command, 0, n)
	i := 0
	e := s.order.Front()
	for i < n {
		if e == nil {
			break
		}
		if c := e.Value.(*cmdElement); !c.proposed {
			cmds = append(cmds, c.cmd)
			i++
		}
		e = e.Next()
	}
	return cmds
}

// Contains returns true if the set contains cmd, false otherwise
func (s *CommandSet) Contains(cmd Command) bool {
	s.mut.Lock()
	defer s.mut.Unlock()
	_, ok := s.set[cmd]
	return ok
}

// Len returns the length of the set
func (s *CommandSet) Len() int {
	s.mut.Lock()
	defer s.mut.Unlock()
	return len(s.set)
}

// TrimToLen will try to remove proposed elements from the set until its length is equal to or less than 'length'
func (s *CommandSet) TrimToLen(length int) {
	s.mut.Lock()
	defer s.mut.Unlock()

	e := s.order.Front()
	for length < len(s.set) {
		if e == nil {
			break
		}
		n := e.Next()
		c := e.Value.(*cmdElement)
		if c.proposed {
			s.order.Remove(e)
			delete(s.set, c.cmd)
		}
		e = n
	}
}

// IsProposed will return true if the given command is marked as proposed
func (s *CommandSet) IsProposed(cmd Command) bool {
	s.mut.Lock()
	defer s.mut.Unlock()
	if e, ok := s.set[cmd]; ok {
		return e.Value.(*cmdElement).proposed
	}
	return false
}

// MarkProposed will mark the given commands as proposed and move them to the back of the queue
func (s *CommandSet) MarkProposed(cmds ...Command) {
	s.mut.Lock()
	defer s.mut.Unlock()
	for _, cmd := range cmds {
		if e, ok := s.set[cmd]; ok {
			e.Value.(*cmdElement).proposed = true
			// Move to back so that it's not immediately deleted by a call to TrimToLen()
			s.order.MoveToBack(e)
		} else {
			// We don't have the command locally yet, so let's store it
			e := s.order.PushBack(&cmdElement{cmd: cmd, proposed: true})
			s.set[cmd] = e
		}
	}
}
