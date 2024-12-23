package config

config: {
	latenciesFile: "latencies/aws.csv"

	replicaHosts: ["relab1"]
	clientHosts: ["relab2"]
	replicas: 5
	clients:  2

	locations: ["paris", "rome", "oslo", "london", "berlin"]
	treePositions: [3, 2, 1, 4, 5]
	branchFactor: 2
}
