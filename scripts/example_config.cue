package config

config: {
	replicaHosts: [
        "hotstuff-worker-2",
        "hotstuff-worker-3",
        "hotstuff-worker-4",
    ]

	clientHosts: [
        "hotstuff-worker-1",
    ]

    replicas: 8
    clients: 2
}
