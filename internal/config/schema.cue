package config

import "list"

config: {
	// File path for latencies between locations (optional).
	latenciesFile?: string & =~".+\\.csv$"

	// List of replica/client hosts (non-empty)
	replicaHosts: [_, ...string]
	clientHosts: [_, ...string]

	// Number of replicas/clients; must be greater than 0
	replicas: int & >0
	clients:  int & >0

	// Both `locations` and `treePositions` must have exactly `replicas` entries and be unique.
	_exactAndUnique: list.MinItems(replicas) & list.MaxItems(replicas) & list.UniqueItems()

	// List of integers representing positions in a tree (optional).
	// Root, left child, right child, left child of left child, etc.
	treePositions?: [...int & >=1 & <=replicas] & _exactAndUnique

	if treePositions == _|_ {
		// List of locations; optional when treePositions is not provided.
		locations?: [...string] & _exactAndUnique
	}
	if treePositions != _|_ {
		// List of locations; required when treePositions is provided.
		locations!: [...string] & _exactAndUnique
	}

	// Byzantine strategy for different replicas (optional).
	byzantineStrategy?: {
		silent?: [...int & >=1 & <=replicas]
		slow?: [...int & >=1 & <=replicas]
	}
}
