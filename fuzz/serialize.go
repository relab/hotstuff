package fuzz

import (
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
)

func fuzzMsgToB64(msg *FuzzMsg) (string, error) {
	bytes, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func b64ToFuzzMsg(str string) (*FuzzMsg, error) {
	bytes, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}
	msg := new(FuzzMsg)
	err = proto.Unmarshal(bytes, msg)
	if err != nil {
		return nil, err
	}
	return msg, err
}

func saveStringToFile(filename string, str string) error {
	return os.WriteFile(filename, []byte(str), 0o600)
}

func loadStringFromFile(filename string) (string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func loadFuzzMessagesFromFile(filename string) ([]*FuzzMsg, error) {
	str, err := loadStringFromFile(filename)
	if err != nil {
		return nil, err
	}
	fuzzMsgs := make([]*FuzzMsg, 0)
	for _, b64 := range strings.Split(str, "\n") {
		if b64 == "" {
			continue
		}
		fuzzMsg, err := b64ToFuzzMsg(b64)
		if err != nil {
			return nil, err
		}
		fuzzMsgs = append(fuzzMsgs, fuzzMsg)
	}
	return fuzzMsgs, nil
}

func loadSeedsFromFile(filename string) ([]int64, error) {
	str, err := loadStringFromFile(filename)
	if err != nil {
		return nil, err
	}
	seeds := make([]int64, 0)
	for _, seedString := range strings.Split(str, "\n") {
		if seedString == "" {
			continue
		}
		seed, err := strconv.ParseInt(seedString, 10, 64)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}
	return seeds, nil
}
