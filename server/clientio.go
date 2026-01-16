package server

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

// clientStatusWindow tracks command statuses for a single client using an array-based approach.
// This is more memory-efficient than nested maps, especially for increasing sequence numbers.
type clientStatusWindow struct {
	baseSeqNum uint64                   // starting sequence number for the current window
	statuses   []hotstuff.CommandStatus // array indexed by (seqNum - baseSeqNum)
}

// CommandStatusTracker efficiently tracks command status per client per sequence number.
// Uses array-based storage per client to avoid the overhead of nested maps.
// Supports sliding window cleanup to prevent unbounded memory growth.
type CommandStatusTracker struct {
	// clientWindows maps ClientID -> client status window
	clientWindows map[uint32]*clientStatusWindow
}

// NewCommandStatusTracker creates a new status tracker
func NewCommandStatusTracker() *CommandStatusTracker {
	return &CommandStatusTracker{
		clientWindows: make(map[uint32]*clientStatusWindow),
	}
}

// SetStatus sets the status of a command
func (cst *CommandStatusTracker) SetStatus(clientID uint32, seqNum uint64, status hotstuff.CommandStatus) {
	window, exists := cst.clientWindows[clientID]
	if !exists {
		// Initialize new window for this client, starting at this sequence number
		window = &clientStatusWindow{
			baseSeqNum: seqNum,
			statuses:   make([]hotstuff.CommandStatus, 5000),
		}
		cst.clientWindows[clientID] = window
		window.statuses[0] = status
		return
	}

	// Check if seqNum is within current window
	if seqNum >= window.baseSeqNum {
		index := seqNum - window.baseSeqNum
		// Extend array if necessary
		if index >= uint64(len(window.statuses)) {
			// Grow array by 50% or enough to fit the new index, whichever is larger
			newLen := len(window.statuses) + len(window.statuses)/2 + 1
			if int(index) >= newLen {
				newLen = int(index) + 1
			}
			newStatuses := make([]hotstuff.CommandStatus, newLen)
			copy(newStatuses, window.statuses)
			window.statuses = newStatuses
		}
		window.statuses[index] = status
	}
	// Ignore updates for seqNum < baseSeqNum (already cleaned up)
}

// GetStatus retrieves the status of a command. Returns StatusExecuted if not found.
func (cst *CommandStatusTracker) GetStatus(clientID uint32, seqNum uint64) hotstuff.CommandStatus {
	window, exists := cst.clientWindows[clientID]
	if !exists {
		return hotstuff.EXECUTED
	}

	// Check if seqNum is within current window
	if seqNum >= window.baseSeqNum && seqNum < window.baseSeqNum+uint64(len(window.statuses)) {
		index := seqNum - window.baseSeqNum
		return window.statuses[index]
	}

	// If outside window (cleaned up or not yet added), assume executed
	return hotstuff.EXECUTED
}

// Cleanup removes entries for sequence numbers less than or equal to the given threshold per client.
// This prevents unbounded memory growth by sliding the window forward.
func (cst *CommandStatusTracker) Cleanup(clientID uint32, upToSeqNum uint64) {
	window, exists := cst.clientWindows[clientID]
	if !exists {
		return
	}

	// Calculate how many entries to remove from the front
	if upToSeqNum >= window.baseSeqNum {
		entriesToRemove := int(upToSeqNum - window.baseSeqNum + 1)
		if entriesToRemove >= len(window.statuses) {
			// Remove entire window, clean up the client entry
			delete(cst.clientWindows, clientID)
			return
		}

		// Slide the window forward
		window.statuses = window.statuses[entriesToRemove:]
		window.baseSeqNum = upToSeqNum + 1
	}
}

// GetClientStatuses returns a snapshot of all statuses for a given client (for testing/debugging)
func (cst *CommandStatusTracker) GetClientStatuses(clientID uint32) map[uint64]hotstuff.CommandStatus {
	window, exists := cst.clientWindows[clientID]
	if !exists {
		return make(map[uint64]hotstuff.CommandStatus)
	}

	snapshot := make(map[uint64]hotstuff.CommandStatus, len(window.statuses))
	for i, status := range window.statuses {
		snapshot[window.baseSeqNum+uint64(i)] = status
	}
	return snapshot
}

// ClientIO serves a client.
type ClientIO struct {
	logger   logging.Logger
	cmdCache *clientpb.CommandCache

	mut      sync.Mutex
	srv      *gorums.Server
	hash     hash.Hash
	cmdCount uint32

	lastExecutedSeqNum map[uint32]uint64     // highest executed sequence number per client ID
	statusTracker      *CommandStatusTracker // tracks status of all commands (executed/aborted/failed)

}

// NewClientIO returns a new client IO server.
func NewClientIO(
	el *eventloop.EventLoop,
	logger logging.Logger,
	cmdCache *clientpb.CommandCache,
	srvOpts ...gorums.ServerOption,
) (srv *ClientIO) {
	srv = &ClientIO{
		logger:   logger,
		cmdCache: cmdCache,

		srv:                gorums.NewServer(srvOpts...),
		hash:               sha256.New(),
		lastExecutedSeqNum: make(map[uint32]uint64),
		statusTracker:      NewCommandStatusTracker(),
	}
	clientpb.RegisterClientServer(srv.srv, srv)
	eventloop.Register(el, func(event clientpb.ExecuteEvent) {
		srv.Exec(event.Batch)
	})
	eventloop.Register(el, func(event clientpb.AbortEvent) {
		srv.Abort(event.Batch)
	})
	return srv
}

func (srv *ClientIO) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.logger.Error(err)
		}
	}()
}

func (srv *ClientIO) Stop() {
	srv.srv.Stop()
}

func (srv *ClientIO) Hash() hash.Hash {
	return srv.hash
}

func (srv *ClientIO) CmdCount() uint32 {
	return srv.cmdCount
}

func (srv *ClientIO) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) {
	srv.cmdCache.Add(cmd)
	srv.statusTracker.SetStatus(cmd.ClientID, cmd.SequenceNumber, hotstuff.UNKNOWN)
	ctx.Release()
}

func (srv *ClientIO) Exec(batch *clientpb.Batch) {
	for _, cmd := range batch.GetCommands() {

		srv.mut.Lock()
		if srv.isDuplicate(cmd) {
			srv.logger.Info("duplicate command found")
			srv.mut.Unlock()
			continue
		}
		srv.lastExecutedSeqNum[cmd.ClientID] = cmd.SequenceNumber
		// Mark command as executed in status tracker
		srv.statusTracker.SetStatus(cmd.ClientID, cmd.SequenceNumber, hotstuff.EXECUTED)
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		srv.mut.Unlock()
	}
	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

func (srv *ClientIO) Abort(batch *clientpb.Batch) {
	for _, cmd := range batch.GetCommands() {
		srv.mut.Lock()
		// Mark command as aborted in status tracker
		srv.statusTracker.SetStatus(cmd.ClientID, cmd.SequenceNumber, hotstuff.FAILED)
		srv.mut.Unlock()
	}
}

// isDuplicate return true if the command has already been executed.
// The caller must hold srv.mut.Lock().
func (srv *ClientIO) isDuplicate(cmd *clientpb.Command) bool {
	seqNum, ok := srv.lastExecutedSeqNum[cmd.ClientID]
	return ok && seqNum >= cmd.SequenceNumber
}

// CleanupOldStatuses removes command status entries that are older than the given sequence number
// for a specific client. This should be called periodically to prevent unbounded memory growth.
func (srv *ClientIO) CleanupOldStatuses(clientID uint32, upToSeqNum uint64) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	srv.statusTracker.Cleanup(clientID, upToSeqNum)
}

func (srv *ClientIO) CommandStatus(_ gorums.ServerCtx, in *clientpb.Command) (resp *clientpb.CommandStatusResponse, err error) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	status := srv.statusTracker.GetStatus(in.ClientID, in.SequenceNumber)
	srv.logger.Infof("Received CommandStatus request (client: %d, sequence: %d, status: %d)", in.ClientID, in.SequenceNumber, status)
	return &clientpb.CommandStatusResponse{
		Status:  clientpb.CommandStatusResponse_Status(status),
		Command: in,
	}, nil
}
