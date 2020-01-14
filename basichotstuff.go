package hotstuff

import (
	"log"
	"strconv"
	"time"

	"github.com/relab/hotstuff/proto"
	"google.golang.org/grpc"
)

var addrs = []string{
	"127.0.0.1:8080",
	"127.0.0.1:8081",
	"127.0.0.1:8082",
}

var viewNumber int32 = 0

var typ = proto.PREPARE

type Config struct {
	config  *proto.Configuration
	manager *proto.Manager
}

var theConfig = Config{}

type Replica struct {
	address string
	leader  Leader
}

//this is a leader, gives high moral yeees.
type Leader struct {
	Majority int
	address  string
}

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

func (l *Leader) BroadcastQF(replies []*proto.Msg) (*proto.Msg, bool) {

	msg := mostCommonMatchingMsgs(replies)

	var qc proto.QuorumCert

	if matchingMsg(*msg, typ, viewNumber) {
		if typ == proto.PREPARE {
			qc.Type = typ
			qc.ViewNumber = viewNumber
			qc.Node = msg.Node
			qc.Sig = sign(replies)
		} else if typ == proto.PRE_COMMIT {

		} else if typ == proto.COMMIT {

		} else if typ == proto.DECIDE {

		}
	}
}

func creatClientConnection(adr string) bool {

	mgr, err := proto.NewManager(addrs, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		//grpc.WithTimeout(50*time.Millisecond),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(500*time.Millisecond),
	)
	if err != nil {
		log.Fatal(err)
	}

	theConfig.manager = mgr

	// Get all all available node ids, 3 nodes
	ids := mgr.NodeIDs()

	// Create a configuration including all nodes
	conf, err := mgr.NewConfiguration(ids, &QSpec{2})
	if err != nil {
		log.Fatalln("error creating read config:", err)
	}

	theConfig.config = conf

	return true
}

func matchingMsg(m proto.Msg, t proto.Type, v int32) bool {

	return m.Type == t && m.ViewNumber == v

}

func matchingQC(qc proto.QuorumCert, t proto.Type, v int32) bool {

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
