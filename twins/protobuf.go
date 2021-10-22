package twins

import "github.com/relab/hotstuff/internal/proto/twinspb"

// ScenarioToProto converts a twins scenario into a protobuf message.
func ScenarioToProto(scenario *Scenario) *twinspb.Scenario {
	nodes := make([]*twinspb.NodeID, 0, len(scenario.Nodes))
	for _, node := range scenario.Nodes {
		nodes = append(nodes, NodeIDToProto(node))
	}
	views := make([]*twinspb.View, 0, len(scenario.Views))
	for _, view := range scenario.Views {
		partitions := make([]*twinspb.Partition, 0, len(view.Partitions))
		for _, partition := range view.Partitions {
			nodes := make([]*twinspb.NodeID, 0, len(partition))
			for node := range partition {
				nodes = append(nodes, NodeIDToProto(node))
			}
			partitions = append(partitions, &twinspb.Partition{
				Nodes: nodes,
			})
		}
		views = append(views, &twinspb.View{
			Leader:     uint32(view.Leader),
			Partitions: partitions,
		})
	}
	return &twinspb.Scenario{
		Nodes: nodes,
		Views: views,
	}
}

// NodeIDToProto converts a twins NodeID into a protobuf message.
func NodeIDToProto(id NodeID) *twinspb.NodeID {
	return &twinspb.NodeID{
		ReplicaID: uint32(id.ReplicaID),
		NetworkID: id.NetworkID,
	}
}
