package basichotstuff

import (
	"encoding/hex"
	"hash/fnv"
	"strconv"

	"github.com/relab/hotstuff/pkg/proto"
)

//this is a leader, gives high moral yeees.

func mostCommonMatchingMsgs(msgs []*proto.Msg) *proto.Msg {

	length := len(msgs)

	matches := 0

	msgMap := make(map[*proto.Msg]int)

	for i := range msgs {
		for j := range msgs {
			if i < length-1-j {
				if comparingMsgs(msgs[i], msgs[length-1-j]) {
					matches++
				}
			} else {
				break
			}

		}
		if _, exist := msgMap[msgs[i]]; exist {
			msgMap[msgs[i]] += matches

		} else {
			msgMap[msgs[i]] = matches
		}
		matches = 0
	}

	var mostMatches *proto.Msg

	high := 0

	for k, v := range msgMap {
		if high < v {
			mostMatches = k
		}
	}

	return mostMatches
}

func sign(signers []*proto.Msg) string {
	signature := ""

	for i := range signers {
		signature += strconv.Itoa(i)
	}
	return signature
}

// the leader acting as a client

func matchingMsg(m *proto.Msg, t proto.Type, v int32) bool {

	return m.Type == t && m.ViewNumber == v

}

func matchingQC(qc *proto.QuorumCert, t proto.Type, v int32) bool {

	return qc.Type == t && qc.ViewNumber == v
}

func comparingMsgs(m1 *proto.Msg, m2 *proto.Msg) bool {
	ok := 0
	if m1.Type == m2.Type && m1.ViewNumber == m2.ViewNumber {
		ok++
	}
	if m1.Node.Command == m2.Node.Command && m1.Node.ParentHash == m2.Node.ParentHash {
		ok++
	}

	if ok == 2 {
		return true
	}
	return false
}

//HashNode converts a node into a hash and retruns the hash as a string.
func HashNode(node *proto.HSNode) string {
	h := fnv.New64()
	return hex.EncodeToString(h.Sum([]byte(node.GetParentHash() + node.GetCommand())))
}
