// Package comm provides interfaces for disseminating proposals and aggregating votes.
package comm

import "github.com/relab/hotstuff"

// Disseminator is an interface for disseminating the proposal from the proposer.
type Disseminator interface {
	// Disseminate disseminates the proposal from the proposer.
	Disseminate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error
}

// Aggregator is an interface for handling incoming proposals and replying with a vote.
type Aggregator interface {
	// Aggregate handles incoming proposals and replies with a vote.
	Aggregate(proposal *hotstuff.ProposeMsg, pc hotstuff.PartialCert) error
}

// Communication is an interface that combines Disseminator and Aggregator for convenience.
type Communication interface {
	Disseminator
	Aggregator
}
