package hotstuff

import (
	"fmt"
	"time"

	"github.com/relab/hotstuff/pkg/proto"
	"google.golang.org/grpc"
)

type hotstuffQSpec struct {
	QuorumSize int
	rc         *ReplicaConfig
}

// ProposeQF takes replies from replica after the leader calls the Propose QC and collects them into a quorum cert
func (h hotstuffQSpec) ProposeQF(req *proto.HSNode, replies []*proto.PartialCert) (*proto.QuorumCert, bool) {
	if len(replies) < h.QuorumSize-1 {
		return nil, false
	}

	qc := CreateQuorumCert(nodeFromProto(req))
	for _, pc := range replies {
		qc.AddPartial(partialCertFromProto(pc))
	}
	// TODO: find a way to avoid checking a signature more than once
	if VerifyQuorumCert(h.rc, qc) {
		// TODO: Find a way to avoid converting back to proto
		return qc.toProto(), true
	}
	return nil, false
}

func StartClient(replica *HotStuff, timeout time.Duration) error {
	mgr, err := proto.NewManager(addressInfo, proto.WithGrpcDialOptions(
		grpc.WithBlock(),
		//grpc.WithTimeout(50*time.Millisecond),
		grpc.WithInsecure(),
	),
		proto.WithDialTimeout(timeout),
	)
	if err != nil {
		return fmt.Errorf("Failed to connect to replicas: %w", err)
	}

	replica.manager = mgr

	// Get all all available node ids
	ids := mgr.NodeIDs()

	// Create a configuration including all nodes
	conf, err := mgr.NewConfiguration(ids, &hotstuffQSpec{replica.QuorumSize, replica.ReplicaConfig})
	if err != nil {
		return err
	}

	replica.config = conf

	return nil
}
