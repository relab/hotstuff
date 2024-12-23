package config_test

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff/internal/config"
)

func TestLoad(t *testing.T) {
	replicaHosts := []string{"bbchain1", "bbchain2", "bbchain3", "bbchain4", "bbchain5", "bbchain6"}
	clientHosts := []string{"bbchain7", "bbchain8"}
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	treePositions := []uint32{10, 2, 3, 4, 5, 6, 7, 8, 9, 1}
	byzantineStrategy := map[string][]uint32{
		"silent": {2, 5},
		"slow":   {4},
	}
	validLocOnlyCfg := &config.Config{
		LatenciesFile: "latencies/aws.csv",
		ReplicaHosts:  replicaHosts,
		ClientHosts:   clientHosts,
		Replicas:      10,
		Clients:       2,
		Locations:     locations,
	}
	validLocTreeCfg := &config.Config{
		LatenciesFile: "latencies/aws.csv",
		ReplicaHosts:  replicaHosts,
		ClientHosts:   clientHosts,
		Replicas:      10,
		Clients:       2,
		Locations:     locations,
		TreePositions: treePositions,
		BranchFactor:  5,
	}
	validLocTreeByzCfg := &config.Config{
		LatenciesFile:     "latencies/aws.csv",
		ReplicaHosts:      replicaHosts,
		ClientHosts:       clientHosts,
		Replicas:          10,
		Clients:           2,
		Locations:         locations,
		TreePositions:     treePositions,
		BranchFactor:      5,
		ByzantineStrategy: byzantineStrategy,
	}
	valid2LocOnlyCfg := &config.Config{
		LatenciesFile: "latencies/aws.csv",
		ReplicaHosts:  []string{"relab1"},
		ClientHosts:   []string{"relab2"},
		Replicas:      3,
		Clients:       2,
		Locations:     []string{"paris", "rome", "oslo"},
	}
	valid2LocTreeCfg := &config.Config{
		LatenciesFile: "latencies/aws.csv",
		ReplicaHosts:  []string{"relab1"},
		ClientHosts:   []string{"relab2"},
		Replicas:      5,
		Clients:       2,
		Locations:     []string{"paris", "rome", "oslo", "london", "berlin"},
		TreePositions: []uint32{3, 2, 1, 4, 5},
		BranchFactor:  2,
	}
	valid2NoLocNoTree := &config.Config{
		ReplicaHosts: []string{"relab1"},
		ClientHosts:  []string{"relab2"},
		Replicas:     3,
		Clients:      2,
	}
	tests := []struct {
		name     string
		filename string
		wantCfg  *config.Config
		wantErr  bool
	}{
		{name: "ValidLocationsOnly", filename: "valid-loc-only.cue", wantCfg: validLocOnlyCfg, wantErr: false},
		{name: "ValidLocationsTree", filename: "valid-loc-tree.cue", wantCfg: validLocTreeCfg, wantErr: false},
		{name: "ValidLocationsTreeByz", filename: "valid-loc-tree-byz.cue", wantCfg: validLocTreeByzCfg, wantErr: false},
		{name: "Valid2LocationsOnly", filename: "valid2-loc-only.cue", wantCfg: valid2LocOnlyCfg, wantErr: false},
		{name: "Valid2LocationsTree", filename: "valid2-loc-tree.cue", wantCfg: valid2LocTreeCfg, wantErr: false},
		{name: "Valid2NoLocationsNoTree", filename: "valid2-no-loc-no-tree.cue", wantCfg: valid2NoLocNoTree, wantErr: false},
		{name: "InvalidLocations", filename: "invalid-loc.cue", wantCfg: nil, wantErr: true},
		{name: "InvalidTree", filename: "invalid-tree.cue", wantCfg: nil, wantErr: true},
		{name: "Invalid2TreeOnly", filename: "invalid-tree-only.cue", wantCfg: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCfg, err := config.Load(filepath.Join("testdata", tt.filename))
			if (err != nil) != tt.wantErr {
				t.Errorf("Load(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(gotCfg, tt.wantCfg); diff != "" {
				t.Errorf("Load(%s) mismatch (-want +got):\n%s", tt.filename, diff)
			}
		})
	}
}
