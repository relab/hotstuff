package config

config: {
	latenciesFile: "latencies/aws.csv"

	replicaHosts: ["relab1"]
	clientHosts: ["relab2"]
	replicas: 3
	clients:  2

	treePositions: [3, 2, 1]
}
