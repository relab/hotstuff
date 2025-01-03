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
		cfg  *config.Config
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
	tests := []struct {
		name string
		cfg  *config.Config
		want config.ReplicaMap
	}{
		{name: "NoReplicasNoHost_____", cfg: newConfig(0, []string{}, locations), want: newReplicaMap(hostMap{})},
		{name: "OneReplicaOneHost____", cfg: newConfig(1, []string{"h1"}, locations), want: newReplicaMap(hostMap{"h1": {1}})},
		{name: "TwoReplicasOneHost___", cfg: newConfig(2, []string{"h1"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}})},
		{name: "TwoReplicasTwoHosts__", cfg: newConfig(2, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1}, "h2": {2}})},
		{name: "ThreeReplicasTwoHosts", cfg: newConfig(3, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3}})},
		{name: "FourReplicasTwoHosts_", cfg: newConfig(4, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3, 4}})},
		{name: "FiveReplicasTwoHosts_", cfg: newConfig(5, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2, 3}, "h2": {4, 5}})},
		{name: "SixReplicasTwoHosts__", cfg: newConfig(6, []string{"h1", "h2"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2, 3}, "h2": {4, 5, 6}})},
		{name: "SixReplicasThreeHosts", cfg: newConfig(6, []string{"h1", "h2", "h3"}, locations), want: newReplicaMap(hostMap{"h1": {1, 2}, "h2": {3, 4}, "h3": {5, 6}})},
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

func newConfig(replicas int, replicaHosts, locations []string) *config.Config {
	return &config.Config{Replicas: replicas, ReplicaHosts: replicaHosts, Locations: locations}
}

func newReplicaMap(hostMap map[string][]uint32) config.ReplicaMap {
	replicaMap := make(config.ReplicaMap)
	for host, ids := range hostMap {
		replicas := make([]*orchestrationpb.ReplicaOpts, 0, len(ids))
		for _, id := range ids {
			replicas = append(replicas, &orchestrationpb.ReplicaOpts{ID: uint32(id)})
		}
		replicaMap[host] = replicas
	}
	return replicaMap
}
