package hotstuff

import (
	"bytes"
	"testing"
)

func TestCanStoreAndRetrieveNode(t *testing.T) {
	nodes := NewMapStorage()
	testNode := &Node{Command: []byte("Hello world")}

	nodes.Put(testNode)
	got, ok := nodes.Get(testNode.Hash())

	if !ok || !bytes.Equal(testNode.Command, got.Command) {
		t.Errorf("Failed to retrieve node from storage.")
	}
}
