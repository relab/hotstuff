package metrics

import "sync"

// This file implements a registry for metrics.
// The purpose of the registry is to make it possible to instantiate multiple metrics based on their names only.
// We distinguish between client metrics and replica metrics, but a single metric could work on both.

var (
	registryMut    sync.Mutex
	clientMetrics  = map[string]func() interface{}{}
	replicaMetrics = map[string]func() interface{}{}
)

// RegisterClientMetric registers the constructor of a client metric.
func RegisterClientMetric(name string, constructor func() interface{}) {
	registryMut.Lock()
	defer registryMut.Unlock()
	clientMetrics[name] = constructor
}

// RegisterReplicaMetric registers the constructor of a replica metric.
func RegisterReplicaMetric(name string, constructor func() interface{}) {
	registryMut.Lock()
	defer registryMut.Unlock()
	replicaMetrics[name] = constructor
}

// GetClientMetrics constructs a new instance of each named metric.
func GetClientMetrics(names ...string) (metrics []interface{}) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := clientMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}

// GetReplicaMetrics constructs a new instance of each named metric.
func GetReplicaMetrics(names ...string) (metrics []interface{}) {
	registryMut.Lock()
	defer registryMut.Unlock()

	for _, name := range names {
		if constructor, ok := replicaMetrics[name]; ok {
			metrics = append(metrics, constructor())
		}
	}
	return
}
