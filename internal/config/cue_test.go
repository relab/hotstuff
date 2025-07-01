package config_test

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff/internal/config"
)

func TestNewCue(t *testing.T) {
	replicaHosts := []string{"bbchain1", "bbchain2", "bbchain3", "bbchain4", "bbchain5", "bbchain6"}
	clientHosts := []string{"bbchain7", "bbchain8"}
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	locationsWithDuplicates := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Melbourne", "Toronto", "Prague", "Paris", "Tokyo"}
	treePositions := []uint32{10, 2, 3, 4, 5, 6, 7, 8, 9, 1}
	byzantineStrategy := map[string][]uint32{
		"silent": {2, 5},
		"slow":   {4},
	}
	validLocOnlyCfg := &config.ExperimentConfig{
		ReplicaHosts: replicaHosts,
		ClientHosts:  clientHosts,
		Replicas:     10,
		Clients:      2,
		Locations:    locations,
	}
	validLocOnlyDupEntriesCfg := &config.ExperimentConfig{
		ReplicaHosts: replicaHosts,
		ClientHosts:  clientHosts,
		Replicas:     10,
		Clients:      2,
		Locations:    locationsWithDuplicates,
	}
	validLocTreeCfg := &config.ExperimentConfig{
		ReplicaHosts:  replicaHosts,
		ClientHosts:   clientHosts,
		Replicas:      10,
		Clients:       2,
		Locations:     locations,
		TreePositions: treePositions,
		BranchFactor:  5,
	}
	validLocTreeByzCfg := &config.ExperimentConfig{
		ReplicaHosts:      replicaHosts,
		ClientHosts:       clientHosts,
		Replicas:          10,
		Clients:           2,
		Locations:         locations,
		TreePositions:     treePositions,
		BranchFactor:      5,
		ByzantineStrategy: byzantineStrategy,
	}
	valid2LocOnlyCfg := &config.ExperimentConfig{
		ReplicaHosts: []string{"relab1"},
		ClientHosts:  []string{"relab2"},
		Replicas:     3,
		Clients:      2,
		Locations:    []string{"paris", "rome", "oslo"},
	}
	valid2LocTreeCfg := &config.ExperimentConfig{
		ReplicaHosts:  []string{"relab1"},
		ClientHosts:   []string{"relab2"},
		Replicas:      5,
		Clients:       2,
		Locations:     []string{"paris", "rome", "oslo", "london", "berlin"},
		TreePositions: []uint32{3, 2, 1, 4, 5},
		BranchFactor:  2,
	}
	valid2NoLocNoTree := &config.ExperimentConfig{
		ReplicaHosts: []string{"relab1"},
		ClientHosts:  []string{"relab2"},
		Replicas:     3,
		Clients:      2,
	}
	exp := &config.ExperimentConfig{
		ReplicaHosts:      []string{"localhost"},
		ClientHosts:       []string{"localhost"},
		Replicas:          4,
		Clients:           1,
		Locations:         []string{"Rome", "Oslo", "London", "Munich"},
		TreePositions:     []uint32{3, 2, 1, 4},
		BranchFactor:      2,
		Consensus:         "chainedhotstuff",
		Communication:     "clique",
		Crypto:            "ecdsa",
		LeaderRotation:    "round-robin",
		ByzantineStrategy: map[string][]uint32{"": {}},
	}
	tests := []struct {
		name     string
		filename string
		wantCfg  *config.ExperimentConfig
		wantErr  bool
	}{
		{name: "ValidLocationsOnly", filename: "valid-loc-only.cue", wantCfg: validLocOnlyCfg, wantErr: false},
		{name: "ValidLocationsDuplicateEntries", filename: "valid-loc-dup-entries.cue", wantCfg: validLocOnlyDupEntriesCfg, wantErr: false},
		{name: "ValidLocationsTree", filename: "valid-loc-tree.cue", wantCfg: validLocTreeCfg, wantErr: false},
		{name: "ValidLocationsTreeByz", filename: "valid-loc-tree-byz.cue", wantCfg: validLocTreeByzCfg, wantErr: false},
		{name: "Valid2LocationsOnly", filename: "valid2-loc-only.cue", wantCfg: valid2LocOnlyCfg, wantErr: false},
		{name: "Valid2LocationsTree", filename: "valid2-loc-tree.cue", wantCfg: valid2LocTreeCfg, wantErr: false},
		{name: "Valid2NoLocationsNoTree", filename: "valid2-no-loc-no-tree.cue", wantCfg: valid2NoLocNoTree, wantErr: false},
		{name: "InvalidLocationsSize", filename: "invalid-loc-size.cue", wantCfg: nil, wantErr: true},
		{name: "InvalidTree", filename: "invalid-tree.cue", wantCfg: nil, wantErr: true},
		{name: "Invalid2TreeOnly", filename: "invalid-tree-only.cue", wantCfg: nil, wantErr: true},
		{name: "Experiments", filename: "exp.cue", wantCfg: exp, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCfg, err := config.NewCue(filepath.Join("testdata", tt.filename), nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCue(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				return
			}
			if diff := cmp.Diff(tt.wantCfg, gotCfg); diff != "" {
				t.Errorf("NewCue(%s) mismatch (-want +got):\n%s", tt.filename, diff)
			}
		})
	}
}

func TestNewCueExperiment(t *testing.T) {
	fourExp := []*config.ExperimentConfig{
		{
			ReplicaHosts:      []string{"localhost"},
			ClientHosts:       []string{"localhost"},
			Replicas:          4,
			Clients:           1,
			Locations:         []string{"Rome", "Oslo", "London", "Munich"},
			TreePositions:     []uint32{3, 2, 1, 4},
			BranchFactor:      2,
			Consensus:         "chainedhotstuff",
			Communication:     "clique",
			Crypto:            "ecdsa",
			LeaderRotation:    "round-robin",
			ByzantineStrategy: map[string][]uint32{"": {}},
		},
		{
			ReplicaHosts:      []string{"localhost"},
			ClientHosts:       []string{"localhost"},
			Replicas:          4,
			Clients:           1,
			Locations:         []string{"Rome", "Oslo", "London", "Munich"},
			TreePositions:     []uint32{3, 2, 1, 4},
			BranchFactor:      2,
			Consensus:         "simplehotstuff",
			Communication:     "clique",
			Crypto:            "ecdsa",
			LeaderRotation:    "round-robin",
			ByzantineStrategy: map[string][]uint32{"fork": {2}},
		},
		{
			ReplicaHosts:      []string{"localhost"},
			ClientHosts:       []string{"localhost"},
			Replicas:          4,
			Clients:           1,
			Locations:         []string{"Rome", "Oslo", "London", "Munich"},
			TreePositions:     []uint32{3, 2, 1, 4},
			BranchFactor:      2,
			Consensus:         "fasthotstuff",
			Communication:     "kauri",
			Crypto:            "ecdsa",
			LeaderRotation:    "round-robin",
			ByzantineStrategy: map[string][]uint32{"silence": {2}},
		},
		{
			ReplicaHosts:      []string{"localhost"},
			ClientHosts:       []string{"localhost"},
			Replicas:          4,
			Clients:           1,
			Locations:         []string{"Rome", "Oslo", "London", "Munich"},
			TreePositions:     []uint32{3, 2, 1, 4},
			BranchFactor:      2,
			Consensus:         "chainedhotstuff",
			Communication:     "kauri",
			Crypto:            "ecdsa",
			LeaderRotation:    "round-robin",
			ByzantineStrategy: map[string][]uint32{"": {}},
		},
	}
	tests := []struct {
		name     string
		filename string
		wantCfg  []*config.ExperimentConfig
		wantLen  int
		wantErr  bool
	}{
		{name: "FourExperiments", filename: "four-experiments.cue", wantCfg: fourExp, wantLen: 4, wantErr: false},
		{name: "SweepExperiments", filename: "sweep-experiments.cue", wantLen: 16, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCfg, err := config.NewCueExperiments(filepath.Join("testdata", tt.filename))
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCueExperiments(%s) error = %v, want %v", tt.filename, err, tt.wantErr)
				return
			}
			if len(gotCfg) != tt.wantLen {
				t.Errorf("NewCueExperiments(%s) length = %d, want %d", tt.filename, len(gotCfg), tt.wantLen)
			}
			if tt.wantCfg == nil {
				return // skip diff check for large parameter sweeps
			}
			if diff := cmp.Diff(tt.wantCfg, gotCfg); diff != "" {
				t.Errorf("NewCueExperiments(%s) mismatch (-want +got):\n%s", tt.filename, diff)
			}
		})
	}
}
