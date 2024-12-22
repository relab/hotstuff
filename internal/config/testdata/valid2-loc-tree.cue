package config

config: {
	latenciesFile: "latencies/aws.csv"

	replicaHosts: ["relab1"]
	clientHosts: ["relab2"]
	replicas: 3
	clients:  2

	locations: ["paris", "rome", "oslo"]
	treePositions: [3, 2, 1]
}
