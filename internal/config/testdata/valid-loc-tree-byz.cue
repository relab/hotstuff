package config

config: {
	latenciesFile: "latencies/aws.csv"

	replicaHosts: [
		"bbchain1",
		"bbchain2",
		"bbchain3",
		"bbchain4",
		"bbchain5",
		"bbchain6",
	]
	clientHosts: [
		"bbchain7",
		"bbchain8",
	]

	replicas: 10
	clients:  2

	locations: ["Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"]
	treePositions: [10, 2, 3, 4, 5, 6, 7, 8, 9, 1]
	branchFactor: 5

	byzantineStrategy: {
		silent: [2, 5]
		slow: [4]
	}
}
