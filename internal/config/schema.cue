package config

import "list"

config: {
	// List of replica/client hosts (non-empty)
	replicaHosts: [_, ...string]
	clientHosts: [_, ...string]

	// Number of replicas/clients; must be greater than 0
	replicas: int & >0
	clients:  int & >0

	// `locations` must have exactly `replicas` entries.
	_exact: list.MinItems(replicas) & list.MaxItems(replicas)
	// `treePositions` must have exactly `replicas` entries and be unique.
	_exactAndUnique: _exact & list.UniqueItems()

	// List of integers representing positions in a tree (optional).
	// Root, left child, right child, left child of left child, etc.
	treePositions?: [...int & >=1 & <=replicas] & _exactAndUnique

	if treePositions == _|_ {
		// List of locations; optional when treePositions is not provided.
		locations?: [...string] & _exact
	}
	if treePositions != _|_ {
		// List of locations; required when treePositions is provided.
		locations!: [...string] & _exact
		// Branching factor of the tree; must be greater than 1 and at most half the number of replicas.
		branchFactor!: int & >1 & <=div(replicas, 2)
	}

	// Consensus algorithm to use. (Default: "chainedhotstuff")
	consensus: *"chainedhotstuff" | "simplehotstuff" | "fasthotstuff"
	// Leader rotation strategy to use. (Default: "round-robin")
	leaderRotation: *"round-robin" | "fixed" | "carousel" | "reputation"
	// Cryptographic algorithm to use. (Default: "ecdsa")
	crypto: *"ecdsa" | "eddsa" | "bls12"
	// Communication protocol to use. (Default: "clique")
	communication: *"clique" | "kauri"

	// Byzantine strategy for different replicas (optional).
	byzantineStrategy?: {
		silent?: [...int & >=1 & <=replicas]
		slow?: [...int & >=1 & <=replicas]
	}
}
