package config_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff/internal/config"
	"github.com/relab/hotstuff/internal/proto/orchestrationpb"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestReplicasForHost(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.ExperimentConfig
		idx  []int
		want []int
	}{
		{name: "NoReplicasNoHost_____", cfg: newConfig(0, []string{}, nil), idx: []int{0}, want: []int{0}},
		{name: "OneReplicaOneHost____", cfg: newConfig(1, []string{"h1"}, nil), idx: []int{0}, want: []int{1}},
		{name: "TwoReplicasOneHost___", cfg: newConfig(2, []string{"h1"}, nil), idx: []int{0}, want: []int{2}},
		{name: "TwoReplicasTwoHosts__", cfg: newConfig(2, []string{"h1", "h2"}, nil), idx: []int{0, 1}, want: []int{1, 1}},
		{name: "ThreeReplicasTwoHosts", cfg: newConfig(3, []string{"h1", "h2"}, nil), idx: []int{0, 1}, want: []int{2, 1}},
		{name: "FourReplicasTwoHosts_", cfg: newConfig(4, []string{"h1", "h2"}, nil), idx: []int{0, 1}, want: []int{2, 2}},
		{name: "FiveReplicasTwoHosts_", cfg: newConfig(5, []string{"h1", "h2"}, nil), idx: []int{0, 1}, want: []int{3, 2}},
		{name: "SixReplicasTwoHosts__", cfg: newConfig(6, []string{"h1", "h2"}, nil), idx: []int{0, 1}, want: []int{3, 3}},
		{name: "SixReplicasThreeHosts", cfg: newConfig(6, []string{"h1", "h2", "h3"}, nil), idx: []int{0, 1, 2}, want: []int{2, 2, 2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, i := range tt.idx {
				if got := tt.cfg.ReplicasForHost(i); got != tt.want[i] {
					t.Errorf("replicasForHost() = %v, want %v", got, tt.want[i])
				}
			}
		})
	}
}

func TestAssignReplicas(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	type hostMap map[string][]uint32
	emptyByz := map[uint32]string{}
	tests := []struct {
		name string
		cfg  *config.ExperimentConfig
		want config.ReplicaMap
	}{
		{name: "NoReplicasNoHost_____", cfg: newConfig(0, []string{}, locations), want: newReplicaMap(hostMap{}, emptyByz)},
		{name: "OneReplicaOneHost____", cfg: newConfig(1, []string{"h1"}, locations), want: newReplicaMap(hostMap{"h1": {1}}, emptyByz)},
		{name: "TwoReplicasOneHost___", cfg: newConfig(2, []string{"h1"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}}, emptyByz)},
		{name: "TwoReplicasTwoHosts__", cfg: newConfig(2, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1}, "h2": {2}}, emptyByz)},
		{name: "ThreeReplicasTwoHosts", cfg: newConfig(3, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3}}, emptyByz)},
		{name: "FourReplicasTwoHosts_", cfg: newConfig(4, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3, 4}}, emptyByz)},
		{name: "FiveReplicasTwoHosts_", cfg: newConfig(5, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2, 3}, "h2": {4, 5}}, emptyByz)},
		{name: "SixReplicasTwoHosts__", cfg: newConfig(6, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2, 3}, "h2": {4, 5, 6}}, emptyByz)},
		{name: "SixReplicasThreeHosts", cfg: newConfig(6, []string{"h1", "h2", "h3"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3, 4}, "h3": {5, 6}}, emptyByz)},
	}
	for _, tt := range tests {
		// Hack to set the locations for the expected output
		for _, replicas := range tt.want {
			for _, r := range replicas {
				r.Locations = locations
			}
		}
		t.Run(tt.name, func(t *testing.T) {
			replicaOpts := &orchestrationpb.ReplicaOpts{}
			got := tt.cfg.AssignReplicas(replicaOpts)
			if diff := cmp.Diff(got, tt.want, protocmp.Transform()); diff != "" {
				t.Errorf("AssignReplicas() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAssignReplicasByzantine(t *testing.T) {
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	byzStrategy := map[string][]uint32{
		"silent": {1, 3},
		"slow":   {2},
	}
	byzMap := map[uint32]string{1: "silent", 2: "slow", 3: "silent"}
	type hostMap map[string][]uint32
	tests := []struct {
		name string
		cfg  *config.ExperimentConfig
		want config.ReplicaMap
	}{
		{name: "ThreeReplicasTwoHosts", cfg: newByzConfig(3, []string{"h1", "h2"}, locations, byzStrategy), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3}}, byzMap)},
		{name: "FourReplicasTwoHosts_", cfg: newByzConfig(4, []string{"h1", "h2"}, locations, byzStrategy), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3, 4}}, byzMap)},
		{name: "FiveReplicasTwoHosts_", cfg: newByzConfig(5, []string{"h1", "h2"}, locations, byzStrategy), want: newReplicaMap(hostMap{"h1": {1, 2, 3}, "h2": {4, 5}}, byzMap)},
		{name: "SixReplicasTwoHosts__", cfg: newByzConfig(6, []string{"h1", "h2"}, locations, byzStrategy), want: newReplicaMap(hostMap{"h1": {1, 2, 3}, "h2": {4, 5, 6}}, byzMap)},
		{name: "SixReplicasThreeHosts", cfg: newByzConfig(6, []string{"h1", "h2", "h3"}, locations, byzStrategy), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3, 4}, "h3": {5, 6}}, byzMap)},
	}
	for _, tt := range tests {
		// Hack to set the locations for the expected output
		for _, replicas := range tt.want {
			for _, r := range replicas {
				r.Locations = locations
			}
		}
		t.Run(tt.name, func(t *testing.T) {
			replicaOpts := &orchestrationpb.ReplicaOpts{}
			got := tt.cfg.AssignReplicas(replicaOpts)
			if diff := cmp.Diff(got, tt.want, protocmp.Transform()); diff != "" {
				t.Errorf("AssignReplicas() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func newConfig(replicas int, replicaHosts, locations []string) *config.ExperimentConfig {
	return &config.ExperimentConfig{Replicas: replicas, ReplicaHosts: replicaHosts, Locations: locations}
}

func newByzConfig(replicas int, replicaHosts, locations []string, byzantineStrategy map[string][]uint32) *config.ExperimentConfig {
	return &config.ExperimentConfig{Replicas: replicas, ReplicaHosts: replicaHosts, Locations: locations, ByzantineStrategy: byzantineStrategy}
}

func newReplicaMap(hostMap map[string][]uint32, byz map[uint32]string) config.ReplicaMap {
	replicaMap := make(config.ReplicaMap)
	for host, ids := range hostMap {
		replicas := make([]*orchestrationpb.ReplicaOpts, 0, len(ids))
		for _, id := range ids {
			replicas = append(replicas, &orchestrationpb.ReplicaOpts{ID: uint32(id), ByzantineStrategy: byz[id]})
		}
		replicaMap[host] = replicas
	}
	return replicaMap
}
