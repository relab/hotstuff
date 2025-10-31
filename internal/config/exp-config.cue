package config

// This file defines a configuration for running experiments with different parameters.
// Use the following command to generate the `experiments.cue` file:
//   cue eval --out cue -e config.experiments exp-config.cue > experiments.cue
config: {
	// Shared static settings
	shared: {
		replicaHosts: ["localhost"]
		clientHosts: ["localhost"]
		replicas: 4
		clients:  1
		locations: ["Rome", "Oslo", "London", "Munich"]
		treePositions: [3, 2, 1, 4]
		branchFactor: 2
	}

	// Parameter sweeps
	params: {
		consensus: ["chainedhotstuff", "simplehotstuff", "fasthotstuff"]
		leaderRotation: ["round-robin", "fixed", "carousel", "reputation"]
		crypto: ["ecdsa", "eddsa", "bls12"]
		communication: ["clique", "kauri"]
		byz: [
			{strategy: "", targets: []},
			{strategy: "fork", targets: [2]},
			{strategy: "silentproposer", targets: [2]},
		]
	}

	// Cross-product into experiments
	experiments: [
		for cs in params.consensus
		for ld in params.leaderRotation
		for cr in params.crypto
		for cm in params.communication
		for bc in params.byz {
			config: {
				shared
				consensus:      cs
				leaderRotation: ld
				crypto:         cr
				communication:  cm
				byzantineStrategy: {(bc.strategy): bc.targets}
			}
		},
	]
}
