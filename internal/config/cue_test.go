package config_test

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/relab/hotstuff/internal/config"
)

func TestExperimentsIteratorSingle(t *testing.T) {
	replicaHosts := []string{"bbchain1", "bbchain2", "bbchain3", "bbchain4", "bbchain5", "bbchain6"}
	clientHosts := []string{"bbchain7", "bbchain8"}
	locations := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Amsterdam", "Auckland", "Moscow", "Stockholm", "London"}
	locationsWithDuplicates := []string{"Melbourne", "Toronto", "Prague", "Paris", "Tokyo", "Melbourne", "Toronto", "Prague", "Paris", "Tokyo"}
	treePositions := []uint32{10, 2, 3, 4, 5, 6, 7, 8, 9, 1}
	byzantineStrategy := map[string][]uint32{
		"silentproposer": {2, 5},
		"fork":           {4},
	}
	defaultModules := &config.ExperimentConfig{
		Consensus:      "chainedhotstuff",
		Communication:  "clique",
		Crypto:         "ecdsa",
		LeaderRotation: "round-robin",
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
			for expConfig, err := range config.Experiments(filepath.Join("testdata", tt.filename), nil) {
				if (err != nil) != tt.wantErr {
					t.Errorf("Experiments(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				}
				if (tt.wantCfg == nil && expConfig != nil) || (tt.wantCfg != nil && expConfig == nil) {
					t.Errorf("Experiments(%s) mismatch: got %v, want %v", tt.filename, expConfig, tt.wantCfg)
				}
				if expConfig == nil && tt.wantCfg == nil {
					return // both nil, no diff to check
				}
				if tt.wantCfg != nil {
					// merge default modules into the wanted config
					tt.wantCfg.Merge(defaultModules)
				}
				if diff := cmp.Diff(tt.wantCfg, expConfig); diff != "" {
					t.Errorf("Experiments(%s) mismatch (-want +got):\n%s", tt.filename, diff)
				}
			}
		})
	}
}

func TestExperimentsIteratorMultiple(t *testing.T) {
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
			ByzantineStrategy: map[string][]uint32{"silentproposer": {2}},
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
		{name: "NoSuchExperiment", filename: "no-such-experiment.cue", wantLen: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expCnt := 0
			for gotCfg, err := range config.Experiments(filepath.Join("testdata", tt.filename), nil) {
				if (err != nil) != tt.wantErr {
					t.Errorf("Experiments(%s) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
				}
				if gotCfg != nil {
					expCnt++ // only count non-nil configs towards the wanted length
				}
				if tt.wantCfg == nil {
					continue // skip diff check for large parameter sweeps
				}
				wantCfg := tt.wantCfg[expCnt-1]
				if diff := cmp.Diff(wantCfg, gotCfg); diff != "" {
					t.Errorf("Experiments(%s) mismatch (-want +got):\n%s", tt.filename, diff)
				}
			}
			if expCnt != tt.wantLen {
				t.Errorf("Experiments(%s) length = %d, want %d", tt.filename, expCnt, tt.wantLen)
			}
		})
	}
}
