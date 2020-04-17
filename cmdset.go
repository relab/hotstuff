package hotstuff

import (
	"container/list"
	"hash"
	"hash/fnv"
	"sync"
)

// cmdSet is a linkedhashset for Commands
type cmdSet struct {
	mut   sync.Mutex
	set   map[Command]*list.Element
	order list.List
	hash  hash.Hash32
}

func newCmdSet() *cmdSet {
	c := &cmdSet{
		set:  make(map[Command]*list.Element),
		hash: fnv.New32a(),
	}
	c.order.Init()
	return c
}

// Add adds cmds to the set
func (s *cmdSet) Add(cmds ...Command) {
	s.mut.Lock()
	defer s.mut.Unlock()

	for _, cmd := range cmds {
		// avoid duplicates
		if _, ok := s.set[cmd]; ok {
			continue
		}
		e := s.order.PushBack(cmd)
		s.set[cmd] = e
	}
}

// Remove removes cmds from the set
func (s *cmdSet) Remove(cmds ...Command) {
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

// GetFirst returns the n first entries in the set
func (s *cmdSet) GetFirst(n int) []Command {
	s.mut.Lock()
	defer s.mut.Unlock()

	cmds := make([]Command, 0, n)
	for i := 0; i < n; i++ {
		e := s.order.Front()
		if e == nil {
			break
		}
		s.order.Remove(e)
		cmd := e.Value.(Command)
		delete(s.set, cmd)
		cmds = append(cmds, cmd)
	}
	return cmds
}
