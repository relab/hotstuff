package fuzz

import (
	"bufio"
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
)

func fuzzMsgToB64(msg *FuzzMsg) (string, error) {
	bytes, err := proto.Marshal(msg)
	str := base64.StdEncoding.EncodeToString(bytes)

	return str, err
}

func b64ToFuzzMsg(str string) (*FuzzMsg, error) {
	bytes, err := base64.StdEncoding.DecodeString(str)

	msg := new(FuzzMsg)
	proto.Unmarshal(bytes, msg)

	return msg, err
}

/*func b64TofuzzMessages(str string) ([]FuzzMsg, error) {

	strLines := strings.Split(strings.ReplaceAll(str, "\r\n", "\n"), "\n")
	for _, msg := range messages {
		str, err := fuzzMsgToB64(msg)
		if err != nil {
			return nil, err
		}
	}

	return full_str, nil
}*/

func saveStringToFile(filename string, str string) error {

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	_, err = w.WriteString(str)
	w.Flush()

	return nil
}

func loadStringFromFile(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}

	fi, err := f.Stat()
	if err != nil {
		return "", err
	}

	bytes := make([]byte, fi.Size())

	f.Read(bytes)

	b64string := string(bytes)

	return b64string, nil
}

func loadFuzzMessagesFromFile(filename string) ([]*FuzzMsg, error) {

	str, err := loadStringFromFile(filename)

	if err != nil {
		return nil, err
	}

	b64s := strings.Split(str, "\n")

	fuzzMsgs := make([]*FuzzMsg, 0)

	for _, b64 := range b64s {

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

	seedstrs := strings.Split(str, "\n")
	seeds := make([]int64, 0)

	for _, seedstr := range seedstrs {
		if seedstr == "" {
			continue
		}

		seed, err := strconv.ParseInt(seedstr, 10, 64)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}

	return seeds, nil
}
