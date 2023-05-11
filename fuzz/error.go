package fuzz

import (
	"fmt"
	reflect "reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type TypeCount map[reflect.Type]int

func (typeCount TypeCount) Add(typ reflect.Type) {
	typeCount[typ]++
}

func (typeCount TypeCount) String(typeTotalCount TypeCount) string {
	keys := make([]reflect.Type, 0)
	for key := range typeCount {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })

	str := ""
	for _, key := range keys {
		str += key.String() + ": " + strconv.Itoa(typeCount[key]) + " / " + strconv.Itoa(typeTotalCount[key]) + "\n"
	}
	return str
}

type PanicInfo struct {
	Err        any
	StackTrace string
	FuzzMsg    string
	FuzzMsgB64 string
	Seed       *int64
	LineNum    int
	TypeCount  TypeCount
}

type ErrorInfo struct {
	messageFile        string
	currentFuzzMsg     *FuzzMsg
	currentFuzzMsgB64  string
	currentFuzzMsgSeed *int64
	errorCount         int
	panics             map[string]PanicInfo
	totalScenarios     int
	failedScenarios    int
	totalMessages      int
	failedMessages     int
	TypePanicCount     TypeCount
	TypeTotalCount     TypeCount
}

func (errorInfo *ErrorInfo) Init() {
	errorInfo.panics = make(map[string]PanicInfo)
	errorInfo.TypeTotalCount = make(TypeCount)
	errorInfo.TypePanicCount = make(TypeCount)
}

func (errorInfo *ErrorInfo) OutputInfo(t *testing.T) {

	b64s := ""
	seeds := ""

	fmt.Println()
	fmt.Println()
	fmt.Println()

	fmt.Println("ERROR INFO")

	keys := make([]string, 0)
	for key := range errorInfo.panics {
		keys = append(keys, key)
	}

	//sorting the keys of the
	sort.Strings(keys)

	for i, key := range keys {
		panicInfo := errorInfo.panics[key]
		b64s += panicInfo.FuzzMsgB64 + "\n"

		if panicInfo.Seed != nil {
			seeds += strconv.FormatInt(*panicInfo.Seed, 10) + "\n"
		}

		fmt.Println()
		fmt.Printf("ERROR NUMBER %d\n", i+1)
		//contains error location, err text and recover point
		fmt.Println(key)
		fmt.Println()
		fmt.Println("crash amounts grouped by type:")
		fmt.Println(panicInfo.TypeCount.String(errorInfo.TypeTotalCount))
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
	fmt.Printf("unique errors found: %d\n", len(errorInfo.panics))
	fmt.Printf("%d runs were errors\n", errorInfo.errorCount)
	fmt.Printf("%d of %d scenarios failed\n", errorInfo.failedScenarios, errorInfo.totalScenarios)
	fmt.Printf("%d of %d messages failed\n", errorInfo.failedMessages, errorInfo.totalMessages)
	fmt.Println()
	fmt.Println("crash amounts grouped by type:")
	fmt.Println(errorInfo.TypePanicCount.String(errorInfo.TypeTotalCount))
}

func (errorInfo *ErrorInfo) AddTotal(fuzzMessage *FuzzMsg, seed *int64) {
	errorInfo.totalMessages++
	errorInfo.currentFuzzMsg = fuzzMessage
	errorInfo.currentFuzzMsgSeed = seed
	typ := reflect.TypeOf(fuzzMessage.Msg())
	errorInfo.TypeTotalCount.Add(typ)
}

func (errorInfo *ErrorInfo) AddPanic(fullStack string, err2 any, info string) {

	simpleStack := SimplifyStack(fullStack)
	identifier := "error location:\t" + simpleStack + "\nerror info:\t" + fmt.Sprint(err2) + "\nrecovered from:\t" + info

	errorInfo.errorCount++

	oldPanic, okPanic := errorInfo.panics[identifier]

	b64, err := fuzzMsgToB64(errorInfo.currentFuzzMsg)
	if err != nil {
		panic(err)
	}

	FuzzMsgString := errorInfo.currentFuzzMsg.Msg().ToString(0)
	newLines := strings.Count(FuzzMsgString, "\n")

	newPanic := PanicInfo{
		Err:        err2,
		StackTrace: fullStack,
		FuzzMsg:    FuzzMsgString,
		FuzzMsgB64: b64,
		Seed:       errorInfo.currentFuzzMsgSeed,
		LineNum:    newLines,
	}

	if okPanic {
		newPanic.TypeCount = oldPanic.TypeCount
	} else {
		newPanic.TypeCount = make(TypeCount)
	}

	oldLines := oldPanic.LineNum
	if !okPanic || newLines < oldLines {
		errorInfo.panics[identifier] = newPanic
	}
	typ := reflect.TypeOf(errorInfo.currentFuzzMsg.Msg())
	errorInfo.panics[identifier].TypeCount.Add(typ)
	errorInfo.TypePanicCount.Add(typ)
}

func SimplifyStack(stack string) string {
	stackLines := strings.Split(strings.ReplaceAll(stack, "\r\n", "\n"), "\n")
	// line 9 tells us where the panic happened, found through testing
	return stackLines[8][1:]
}
