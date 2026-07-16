package server

import (
	"crypto/sha256"
	"hash"
	"net"
	"sync"

	"github.com/relab/gorums"
	"github.com/relab/hotstuff/core/eventloop"
	"github.com/relab/hotstuff/core/logging"
	"github.com/relab/hotstuff/internal/proto/clientpb"
)

const (
	// hotstuffFailedCommandLength is the maximum number of failed sequence numbers to track per client
	hotstuffFailedCommandLength = 100
)

// clientStatusWindow tracks command statuses for a single client.
// It stores the highest successfully executed sequence number and a list of failed sequence numbers.
type clientStatusWindow struct {
	// HighestSuccess is the highest successfully executed sequence number for this client
	HighestSuccess uint64
	// FailedCmds is the list of recent failed sequence numbers (up to maxFailed entries)
	FailedCmds []uint64
}

// CommandStatusTracker stores per-client command status information.
// It tracks the highest successful sequence number and recent failed sequence numbers for each client.
type CommandStatusTracker struct {
	// clientWindows maps ClientID to its status window
	clientWindows map[uint32]*clientStatusWindow
	// maxFailed is the maximum number of failed sequence numbers to track per client
	maxFailed int
}

// ensureWindow returns the status window for clientID, creating it if it doesn't exist.
func (cst *CommandStatusTracker) ensureWindow(clientID uint32) *clientStatusWindow {
	w, ok := cst.clientWindows[clientID]
	if !ok {
		w = &clientStatusWindow{
			FailedCmds: make([]uint64, 0, cst.maxFailed),
		}
		cst.clientWindows[clientID] = w
	}
	return w
}

// addFailed records a failed sequence number in the window.
// If the window is full (maxFailed entries), it removes the oldest entry before adding the new one.
func (cst *CommandStatusTracker) addFailed(client uint32, seq uint64) {
	w := cst.ensureWindow(client)
	// If we have reached maxFailed, remove the oldest entry
	if len(w.FailedCmds) >= cst.maxFailed {
		w.FailedCmds = w.FailedCmds[1:]
	}
	w.FailedCmds = append(w.FailedCmds, seq)
}

// NewCommandStatusTracker creates a new CommandStatusTracker with default settings.
func NewCommandStatusTracker() *CommandStatusTracker {
	return &CommandStatusTracker{
		clientWindows: make(map[uint32]*clientStatusWindow),
		maxFailed:     hotstuffFailedCommandLength,
	}
}

// setSuccess updates the highest successfully executed sequence number for a client.
// It only updates if the new sequence number is higher than the current highest.
func (cst *CommandStatusTracker) setSuccess(clientID uint32, seqNum uint64) {
	w := cst.ensureWindow(clientID)
	if seqNum < w.HighestSuccess {
		return
	}
	w.HighestSuccess = seqNum
}

// GetClientStatuses returns the status window for a given client.
// If the client doesn't exist, it creates and returns an empty window.
func (cst *CommandStatusTracker) GetClientStatuses(clientID uint32) *clientStatusWindow {
	return cst.ensureWindow(clientID)
}

// ClientIO serves client requests and manages command execution tracking.
type ClientIO struct {
	logger   logging.Logger
	cmdCache *clientpb.CommandCache

	mut      sync.Mutex
	srv      *gorums.Server
	hash     hash.Hash
	cmdCount uint32

	lastExecutedSeqNum map[uint32]uint64     // tracks the highest executed sequence number per client ID
	statusTracker      *CommandStatusTracker // tracks command execution status (success/failure) per client
}

// NewClientIO creates and returns a new ClientIO server instance.
// It registers the server with the event loop to handle Execute and Abort events.
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

// StartOnListener starts the gRPC server on the provided listener.
func (srv *ClientIO) StartOnListener(lis net.Listener) {
	go func() {
		err := srv.srv.Serve(lis)
		if err != nil {
			srv.logger.Error(err)
		}
	}()
}

// Stop stops the gRPC server.
func (srv *ClientIO) Stop() {
	srv.srv.Stop()
}

// Hash returns the current hash of all executed commands.
func (srv *ClientIO) Hash() hash.Hash {
	return srv.hash
}

// CmdCount returns the total number of executed commands.
func (srv *ClientIO) CmdCount() uint32 {
	return srv.cmdCount
}

// ExecCommand receives a command from a client and adds it to the command cache.
func (srv *ClientIO) ExecCommand(ctx gorums.ServerCtx, cmd *clientpb.Command) {
	srv.cmdCache.Add(cmd)
	ctx.Release()
}

// Exec executes a batch of commands, updating the hash and command count.
// It skips duplicate commands and marks successful executions in the status tracker.
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
		srv.statusTracker.setSuccess(cmd.ClientID, cmd.SequenceNumber)
		_, _ = srv.hash.Write(cmd.Data)
		srv.cmdCount++
		srv.mut.Unlock()
	}
	srv.logger.Debugf("Hash: %.8x", srv.hash.Sum(nil))
}

// Abort marks a batch of commands as failed in the status tracker.
func (srv *ClientIO) Abort(batch *clientpb.Batch) {
	for _, cmd := range batch.GetCommands() {
		srv.mut.Lock()
		// Mark command as aborted in status tracker
		srv.statusTracker.addFailed(cmd.ClientID, cmd.SequenceNumber)
		srv.mut.Unlock()
	}
}

// isDuplicate returns true if the command has already been executed.
// The caller must hold srv.mut.Lock().
func (srv *ClientIO) isDuplicate(cmd *clientpb.Command) bool {
	seqNum, ok := srv.lastExecutedSeqNum[cmd.ClientID]
	return ok && seqNum >= cmd.SequenceNumber
}

// CommandStatus returns the execution status for a given command.
// It returns the highest executed sequence number and list of failed sequence numbers for the client.
func (srv *ClientIO) CommandStatus(_ gorums.ServerCtx, in *clientpb.Command) (resp *clientpb.CommandStatusResponse, err error) {
	srv.mut.Lock()
	defer srv.mut.Unlock()
	CommandStatus := srv.statusTracker.GetClientStatuses(in.ClientID)

	return &clientpb.CommandStatusResponse{
		HighestSequenceNumber: CommandStatus.HighestSuccess,
		FailedSequenceNumbers: CommandStatus.FailedCmds,
	}, nil
}
