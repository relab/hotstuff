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
