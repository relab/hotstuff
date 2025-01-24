package config

config: {
	replicaHosts: ["localhost"]
	clientHosts: ["localhost"]
	replicas: 5
	clients:  2

	locations: ["Paris", "Rome", "Oslo", "London", "Munich"]
	treePositions: [3, 2, 1, 4, 5]
	branchFactor: 2
	byzantineStrategy: {
		silence: [1, 2]
		fork: [3]
	}
}
