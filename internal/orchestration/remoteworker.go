package orchestration

import (
	"fmt"

	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"github.com/relab/hotstuff/internal/protostream"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// RemoteWorker is a proxy for a remote worker.
type RemoteWorker struct {
	send *protostream.Writer
	recv *protostream.Reader
}

// NewRemoteWorker returns a new remote worker proxy.
func NewRemoteWorker(send *protostream.Writer, recv *protostream.Reader) RemoteWorker {
	return RemoteWorker{
		send: send,
		recv: recv,
	}
}

func (w RemoteWorker) rpc(req proto.Message) (res proto.Message, err error) {
	err = w.send.WriteAny(req)
	if err != nil {
		return nil, err
	}
	res, err = w.recv.ReadAny()
	if err != nil {
		return nil, err
	}
	// unpack status errors
	if s, ok := res.(*spb.Status); ok {
		return nil, status.FromProto(s).Err()
	}
	return res, nil
}

// CreateReplica requests that the remote worker creates the specified replicas,
// returning details about the created replicas.
func (w RemoteWorker) CreateReplica(req *orchestrationpb.CreateReplicaRequest) (res *orchestrationpb.CreateReplicaResponse, err error) {
	msg, err := w.rpc(req)
	if err != nil {
		return nil, err
	}
	res, ok := msg.(*orchestrationpb.CreateReplicaResponse)
	if !ok {
		return nil, fmt.Errorf("wrong type for response message: got %T, wanted: %T", msg, res)
	}
	return res, nil
}

// StartReplica requests that the remote worker starts the specified replicas.
func (w RemoteWorker) StartReplica(req *orchestrationpb.StartReplicaRequest) (res *orchestrationpb.StartReplicaResponse, err error) {
	msg, err := w.rpc(req)
	if err != nil {
		return nil, err
	}
	res, ok := msg.(*orchestrationpb.StartReplicaResponse)
	if !ok {
		return nil, fmt.Errorf("wrong type for response message: got %T, wanted: %T", msg, res)
	}
	return res, nil
}

// StopReplica requests that the remote worker stops the specified replica.
func (w RemoteWorker) StopReplica(req *orchestrationpb.StopReplicaRequest) (res *orchestrationpb.StopReplicaResponse, err error) {
	msg, err := w.rpc(req)
	if err != nil {
		return nil, err
	}
	res, ok := msg.(*orchestrationpb.StopReplicaResponse)
	if !ok {
		return nil, fmt.Errorf("wrong type for response message: got %T, wanted: %T", msg, res)
	}
	return res, nil
}

// StartClient requests that the remote worker starts the specified clients.
func (w RemoteWorker) StartClient(req *orchestrationpb.StartClientRequest) (res *orchestrationpb.StartClientResponse, err error) {
	msg, err := w.rpc(req)
	if err != nil {
		return nil, err
	}
	res, ok := msg.(*orchestrationpb.StartClientResponse)
	if !ok {
		return nil, fmt.Errorf("wrong type for response message: got %T, wanted: %T", msg, res)
	}
	return res, nil
}

// StopClient requests that the remote worker stops the specified clients.
func (w RemoteWorker) StopClient(req *orchestrationpb.StopClientRequest) (res *orchestrationpb.StopClientResponse, err error) {
	msg, err := w.rpc(req)
	if err != nil {
		return nil, err
	}
	res, ok := msg.(*orchestrationpb.StopClientResponse)
	if !ok {
		return nil, fmt.Errorf("wrong type for response message: got %T, wanted: %T", msg, res)
	}
	return res, nil
}

// Quit requests that the remote worker exits.
func (w RemoteWorker) Quit() (err error) {
	return w.send.WriteAny(&orchestrationpb.QuitRequest{})
}
