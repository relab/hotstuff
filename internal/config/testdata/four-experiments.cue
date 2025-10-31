[{
	config: {
		consensus:      "chainedhotstuff"
		leaderRotation: "round-robin"
		crypto:         "ecdsa"
		communication:  "clique"
		byzantineStrategy: {
			"": []
		}
		replicaHosts: ["localhost"]
		clientHosts: ["localhost"]
		replicas: 4
		clients:  1
		locations: ["Rome", "Oslo", "London", "Munich"]
		treePositions: [3, 2, 1, 4]
		branchFactor: 2
	}
}, {
	config: {
		consensus:      "simplehotstuff"
		leaderRotation: "round-robin"
		crypto:         "ecdsa"
		communication:  "clique"
		byzantineStrategy: {
			fork: [2]
		}
		replicaHosts: ["localhost"]
		clientHosts: ["localhost"]
		replicas: 4
		clients:  1
		locations: ["Rome", "Oslo", "London", "Munich"]
		treePositions: [3, 2, 1, 4]
		branchFactor: 2
	}
}, {
	config: {
		consensus:      "fasthotstuff"
		leaderRotation: "round-robin"
		crypto:         "ecdsa"
		communication:  "kauri"
		byzantineStrategy: {
			silentproposer: [2]
		}
		replicaHosts: ["localhost"]
		clientHosts: ["localhost"]
		replicas: 4
		clients:  1
		locations: ["Rome", "Oslo", "London", "Munich"]
		treePositions: [3, 2, 1, 4]
		branchFactor: 2
	}
}, {
	config: {
		consensus:      "chainedhotstuff"
		leaderRotation: "round-robin"
		crypto:         "ecdsa"
		communication:  "kauri"
		byzantineStrategy: {
			"": []
		}
		replicaHosts: ["localhost"]
		clientHosts: ["localhost"]
		replicas: 4
		clients:  1
		locations: ["Rome", "Oslo", "London", "Munich"]
		treePositions: [3, 2, 1, 4]
		branchFactor: 2
	}
}]
