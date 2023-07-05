package fuzz

import (
	"fmt"
	"math/rand"
	"runtime/debug"
	"testing"

	_ "github.com/relab/hotstuff/consensus/chainedhotstuff"
)

func tryExecuteScenario(errInfo *errorInfo, oldMessage any, newMessage any) {
	errInfo.totalScenarios++
	defer func() {
		if err := recover(); err != nil {
			stack := string(debug.Stack())

			errInfo.addPanic(stack, err, "TryExecuteScenario")
			errInfo.failedScenarios++
		}
	}()

	var numNodes uint8 = 4

	allNodesSet := make(NodeSet)
	for i := 1; i <= int(numNodes); i++ {
		allNodesSet.Add(uint32(i))
	}

	s := Scenario{}
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})
	s = append(s, View{Leader: 1, Partitions: []NodeSet{allNodesSet}})

	result, err := ExecuteScenario(s, numNodes, 0, 100, "chainedhotstuff", oldMessage, newMessage)
	if err != nil {
		panic(err)
	}

	if !result.Safe {
		panic("Expected no safety violations")
	}

	if result.Commits != 1 {
		panic(fmt.Sprintf("Expected one commit (got %d)", result.Commits))
	}
}

func fuzzScenario(errInfo *errorInfo, newMessage any) {
	tryExecuteScenario(errInfo, 1, newMessage)
}

func fuzzMsgToMsg(errInfo *errorInfo, fuzzMsg *FuzzMsg) any {
	errInfo.totalMessages++
	defer func() {
		if err := recover(); err != nil {
			stack := string(debug.Stack())

			errInfo.addPanic(stack, err, "fuzzMsgToMsg")
			errInfo.failedMessages++
		}
	}()
	return fuzzMsgToHotStuffMsg(fuzzMsg)
}

func useFuzzMessage(errInfo *errorInfo, fuzzMessage *FuzzMsg, seed *int64) {
	errInfo.addTotal(fuzzMessage, seed)
	newMessage := fuzzMsgToMsg(errInfo, fuzzMessage)
	if newMessage != nil {
		fuzzScenario(errInfo, newMessage)
	}
}

func ShowProgress(t *testing.T, numIteration int, iterations int, errorCount int) {
	t.Helper()
	fmt.Printf("running test %4d/%4d %4d errors \r", numIteration, iterations, errorCount)
}

// the main test
func TestFuzz(t *testing.T) {
	errInfo := new(errorInfo)
	errInfo.init()

	f := initFuzz()

	iterations := 1000

	for i := 0; i < iterations; i++ {
		ShowProgress(t, i+1, iterations, errInfo.errorCount)

		seed := rand.Int63()
		fuzzMessage := createFuzzMessage(f, &seed)
		useFuzzMessage(errInfo, fuzzMessage, &seed)
	}

	errInfo.outputInfo(t)
}

// load previously created fuzz messages from a file
// it doesn't work quite right, i blame proto.Marshal()
func TestPreviousFuzz(t *testing.T) {
	errInfo := new(errorInfo)
	errInfo.init()

	fuzzMsgs, err := loadFuzzMessagesFromFile("previous_messages.b64")
	if err != nil {
		panic(err)
	}

	for _, fuzzMessage := range fuzzMsgs {
		useFuzzMessage(errInfo, fuzzMessage, nil)
	}

	errInfo.outputInfo(t)
}

// load previously created fuzz messages from a file
// it recreates the fuzz messages from a 64-bit source
func TestSeedPreviousFuzz(t *testing.T) {
	errInfo := new(errorInfo)
	errInfo.init()

	seeds, err := loadSeedsFromFile("previous_messages.seed")
	if err != nil {
		panic(err)
	}

	f := initFuzz()

	for _, seed := range seeds {
		fuzzMessage := createFuzzMessage(f, &seed)
		useFuzzMessage(errInfo, fuzzMessage, nil)
	}

	errInfo.outputInfo(t)
}
