package fuzz

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/meling/proto2"
)

type typeCount map[reflect.Type]int

func (tc typeCount) Add(typ reflect.Type) {
	tc[typ]++
}

func (tc typeCount) String(typeTotalCount typeCount) string {
	keys := make([]reflect.Type, 0)
	for key := range tc {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	str := ""
	for _, key := range keys {
		str += key.String() + ": " + strconv.Itoa(tc[key]) + " / " + strconv.Itoa(typeTotalCount[key]) + "\n"
	}
	return str
}

type panicInfo struct {
	Err        any
	StackTrace string
	FuzzMsg    string
	FuzzMsgB64 string
	Seed       *int64
	LineNum    int
	TypeCount  typeCount
}

type errorInfo struct {
	messageFile        string
	currentFuzzMsg     *FuzzMsg
	currentFuzzMsgB64  string
	currentFuzzMsgSeed *int64
	errorCount         int
	panics             map[string]panicInfo
	totalScenarios     int
	failedScenarios    int
	totalMessages      int
	failedMessages     int
	TypePanicCount     typeCount
	TypeTotalCount     typeCount
}

func (ei *errorInfo) init() {
	ei.panics = make(map[string]panicInfo)
	ei.TypeTotalCount = make(typeCount)
	ei.TypePanicCount = make(typeCount)
}

func (ei *errorInfo) outputInfo(t *testing.T) {
	b64s := ""
	seeds := ""

	fmt.Println()
	fmt.Println()
	fmt.Println()

	fmt.Println("ERROR INFO")

	keys := make([]string, 0)
	for key := range ei.panics {
		keys = append(keys, key)
	}

	// sorting the keys of the
	sort.Strings(keys)

	for i, key := range keys {
		panicInfo := ei.panics[key]
		b64s += panicInfo.FuzzMsgB64 + "\n"

		if panicInfo.Seed != nil {
			seeds += strconv.FormatInt(*panicInfo.Seed, 10) + "\n"
		}

		fmt.Println()
		fmt.Printf("ERROR NUMBER %d\n", i+1)
		// contains error location, err text and recover point
		fmt.Println(key)
		fmt.Println()
		fmt.Println("crash amounts grouped by type:")
		fmt.Println(panicInfo.TypeCount.String(ei.TypeTotalCount))
		fmt.Println("- STACK TRACE BEGIN")
		fmt.Print(panicInfo.StackTrace)
		fmt.Println("- STACK TRACE END")
		fmt.Println()
		fmt.Println("- FUZZ MESSAGE BEGIN")
		fmt.Println(panicInfo.FuzzMsg)
		fmt.Println("- FUZZ MESSAGE END")
		fmt.Println()

		if t != nil {
			t.Error(panicInfo.Err)
		}
	}

	saveStringToFile("previous_messages.b64", b64s)

	if seeds != "" {
		saveStringToFile("previous_messages.seed", seeds)
	}

	fmt.Println()
	fmt.Println("SUMMARY")
	fmt.Printf("unique errors found: %d\n", len(ei.panics))
	fmt.Printf("%d runs were errors\n", ei.errorCount)
	fmt.Printf("%d of %d scenarios failed\n", ei.failedScenarios, ei.totalScenarios)
	fmt.Printf("%d of %d messages failed\n", ei.failedMessages, ei.totalMessages)
	fmt.Println()
	fmt.Println("crash amounts grouped by type:")
	fmt.Println(ei.TypePanicCount.String(ei.TypeTotalCount))
}

func (ei *errorInfo) addTotal(fuzzMessage *FuzzMsg, seed *int64) {
	ei.totalMessages++
	ei.currentFuzzMsg = fuzzMessage
	ei.currentFuzzMsgSeed = seed
	protoMsg := extractProtoMsg(ei.currentFuzzMsg)
	typ := reflect.TypeOf(protoMsg)
	ei.TypeTotalCount.Add(typ)
}

func (ei *errorInfo) addPanic(fullStack string, err2 any, info string) {
	simpleStack := simplifyStack(fullStack)
	identifier := "error location:\t" + simpleStack + "\nerror info:\t" + fmt.Sprint(err2) + "\nrecovered from:\t" + info

	ei.errorCount++

	oldPanic, okPanic := ei.panics[identifier]

	b64, err := fuzzMsgToB64(ei.currentFuzzMsg)
	if err != nil {
		panic(err)
	}

	protoMsg := extractProtoMsg(ei.currentFuzzMsg)
	fuzzMsgString := proto2.GoString(protoMsg)
	newLines := strings.Count(fuzzMsgString, "\n")

	newPanic := panicInfo{
		Err:        err2,
		StackTrace: fullStack,
		FuzzMsg:    fuzzMsgString,
		FuzzMsgB64: b64,
		Seed:       ei.currentFuzzMsgSeed,
		LineNum:    newLines,
	}

	if okPanic {
		newPanic.TypeCount = oldPanic.TypeCount
	} else {
		newPanic.TypeCount = make(typeCount)
	}

	oldLines := oldPanic.LineNum
	if !okPanic || newLines < oldLines {
		ei.panics[identifier] = newPanic
	}
	typ := reflect.TypeOf(protoMsg)
	ei.panics[identifier].TypeCount.Add(typ)
	ei.TypePanicCount.Add(typ)
}

func simplifyStack(stack string) string {
	stackLines := strings.Split(strings.ReplaceAll(stack, "\r\n", "\n"), "\n")
	// line 9 tells us where the panic happened, found through testing
	return stackLines[8][1:]
}
