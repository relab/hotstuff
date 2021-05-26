package orchestration

import (
	"context"

	"github.com/relab/hotstuff/consensus"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
)

type client struct{}

type worker struct {
	replicas map[consensus.ID]replica.Replica
	clients  map[consensus.ID]client
}

func (w *worker) CreateReplica(ctx context.Context, cfg *orchestrationpb.CreateReplicaRequest, ret func(*orchestrationpb.CreateReplicaResponse, error)) {

}

func (w *worker) CreateClient(_ context.Context, _ *orchestrationpb.CreateClientRequest, _ func(*orchestrationpb.CreateClientResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func (w *worker) StartReplica(_ context.Context, _ *orchestrationpb.StartReplicaRequest, _ func(*orchestrationpb.StartReplicaResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func (w *worker) StartClient(_ context.Context, _ *orchestrationpb.StartClientRequest, _ func(*orchestrationpb.StartClientResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func (w *worker) StopReplica(_ context.Context, _ *orchestrationpb.StopReplicaRequest, _ func(*orchestrationpb.StopReplicaResponse, error)) {
	panic("not implemented") // TODO: Implement
}

func (w *worker) StopClient(_ context.Context, _ *orchestrationpb.StopClientRequest, _ func(*orchestrationpb.StopClientResponse, error)) {
	panic("not implemented") // TODO: Implement
}
