package config

import (
	_ "embed"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

//go:embed schema.cue
var schemaFile string

// NewCue loads a cue configuration from filename and returns a ExperimentConfig struct.
// The configuration is validated against the schema embedded in the binary.
// One can specify the `base` config to overwrite its values from the Cue config.
// Leave `base` nil to construct a Cue config from scratch.
func NewCue(filename string, base *ExperimentConfig) (*ExperimentConfig, error) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(schemaFile).LookupPath(cue.ParsePath("config"))
	if schema.Err() != nil {
		return nil, schema.Err()
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg := ctx.CompileString(string(b)).LookupPath(cue.ParsePath("config"))
	if cfg.Err() != nil {
		return nil, cfg.Err()
	}
	unified := schema.Unify(cfg)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}

	if base == nil {
		base = &ExperimentConfig{}
	}

	if err := cfg.Decode(base); err != nil {
		return nil, err
	}
	return base, nil
}
