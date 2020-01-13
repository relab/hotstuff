package hotstuff

import (
	"log"
	"time"

	"github.com/relab/hotstuff/proto"
	"google.golang.org/grpc"
)

var addrs = []string{
	"127.0.0.1:8080",
	"127.0.0.1:8081",
	"127.0.0.1:8082",
}

type Config struct {

	//config *Configuration,

}

type Replica struct {
}

type Leader struct {
	Majority int
}


func allMatchingMsgs(msgs []*proto.Msg) []*proto.Msg {
	
} 


func (l *Leader) BroadcastQF(replies []*proto.Msg) (*proto.Msg, bool) {
	
	replies[0].
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

	// Get all all available node ids, 3 nodes
	ids := mgr.NodeIDs()

	// Create a configuration including all nodes
	allNodesConfig, err := mgr.NewConfiguration(ids, &QSpec{2})
	if err != nil {
		log.Fatalln("error creating read config:", err)
	}

}

func matchingMsg(m proto.Msg, t proto.Type, v int32) bool {

	return m.Type == t && m.ViewNumber == v

}

func matchingQC(qc proto.QuorumCert, t proto.Type, v int32) bool {

	return qc.Type == t && qc.ViewNumber == v
}
