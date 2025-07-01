package config

import (
	_ "embed"
	"errors"
	"fmt"
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

func NewCueExperiments(filename string) ([]*ExperimentConfig, error) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(schemaFile).LookupPath(cue.ParsePath("config"))
	if schema.Err() != nil {
		return nil, schema.Err()
	}
	// read and compile the experiments file (top-level array)
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	experimentList := ctx.CompileString(string(b), cue.Filename(filename))
	if experimentList.Err() != nil {
		return nil, experimentList.Err()
	}
	var head cue.Value
	var errMissing error
	// lookup the experiments in the compiled file
	expandList := experimentList.LookupPath(cue.ParsePath("config.experiments"))
	if expandList.Err() != nil {
		errMissing = expandList.Err() // missing config.experiments
		head = experimentList.Value() // file may contain a list of experiments
	} else {
		head = expandList.Value() // file may expanded into a list of experiments
	}

	// walk the list
	iter, err := head.List()
	if err != nil {
		return nil, fmt.Errorf("experiments file must be or expand into an array: %w", errors.Join(errMissing, err))
	}
	var out []*ExperimentConfig
	for iter.Next() {
		elem := iter.Value() // { config: { … } }
		cfg := elem.LookupPath(cue.ParsePath("config"))
		if cfg.Err() != nil {
			return nil, cfg.Err()
		}
		unified := schema.Unify(cfg)
		if err := unified.Validate(cue.Concrete(true)); err != nil {
			return nil, err
		}
		var ec ExperimentConfig
		if err := unified.Decode(&ec); err != nil {
			return nil, err
		}
		out = append(out, &ec)
	}

	return out, nil
}
