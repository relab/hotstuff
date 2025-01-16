package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/relab/hotstuff/internal/tree"
	"github.com/spf13/viper"
)

func NewViper() (*HostConfig, error) {
	intTreePos := viper.GetIntSlice("tree-pos")
	treePos := make([]uint32, len(intTreePos))
	for i, pos := range intTreePos {
		treePos[i] = uint32(pos)
	}

	cfg := &HostConfig{
		Duration:            viper.GetDuration("duration"),
		Replicas:            viper.GetInt("replicas"),
		Clients:             viper.GetInt("clients"),
		ReplicaHosts:        viper.GetStringSlice("replica-hosts"),
		ClientHosts:         viper.GetStringSlice("client-hosts"),
		BranchFactor:        viper.GetUint32("bf"),
		TreeDelta:           viper.GetDuration("tree-delta"),
		RandomTree:          viper.GetBool("random-tree"),
		Output:              viper.GetString("output"),
		TreePositions:       treePos,
		Worker:              viper.GetBool("worker"),
		Exe:                 viper.GetString("exe"),
		SshConfig:           viper.GetString("ssh-config"),
		LogLevel:            viper.GetString("log-level"),
		CpuProfile:          viper.GetBool("cpu-profile"),
		MemProfile:          viper.GetBool("mem-profile"),
		Trace:               viper.GetBool("trace"),
		FgProfProfile:       viper.GetBool("fgprof-profile"),
		Metrics:             viper.GetStringSlice("metrics"),
		MeasurementInterval: viper.GetDuration("measurement-interval"),
		BatchSize:           viper.GetUint32("batch-size"),
		TimeoutMultiplier:   viper.GetFloat64("timeout-multiplier"),
		Consensus:           viper.GetString("consensus"),
		Crypto:              viper.GetString("crypto"),
		LeaderRotation:      viper.GetString("leader-rotation"),
		ConnectTimeout:      viper.GetDuration("connect-timeout"),
		ViewTimeout:         viper.GetDuration("view-timeout"),
		DurationSamples:     viper.GetUint32("duration-samples"),
		MaxTimeout:          viper.GetDuration("max-timeout"),
		SharedSeed:          viper.GetInt64("shared-seed"),
		Modules:             viper.GetStringSlice("modules"),
		PayloadSize:         viper.GetUint32("payload-size"),
		MaxConcurrent:       viper.GetUint32("max-concurrent"),
		RateLimit:           viper.GetFloat64("rate-limit"),
		RateStep:            viper.GetFloat64("rate-step"),
		RateStepInterval:    viper.GetDuration("rate-step-interval"),
		ClientTimeout:       viper.GetDuration("client-timeout"),
	}

	if len(cfg.ReplicaHosts) == 0 {
		cfg.ReplicaHosts = []string{"localhost"}
	}

	if len(cfg.ClientHosts) == 0 {
		cfg.ClientHosts = []string{"localhost"}
	}

	var err error
	if cfg.Output != "" {
		cfg.Output, err = filepath.Abs(cfg.Output)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %v", err)
		}

		err = os.MkdirAll(cfg.Output, 0o755)
		if err != nil {
			return nil, fmt.Errorf("failed to create output directory: %v", err)
		}
	}

	if len(cfg.TreePositions) == 0 {
		cfg.TreePositions = tree.DefaultTreePosUint32(cfg.Replicas)
	}

	return cfg, nil
}
