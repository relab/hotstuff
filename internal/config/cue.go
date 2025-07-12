package config

import (
	_ "embed"
	"errors"
	"fmt"
	"iter"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

//go:embed schema.cue
var schemaFile string

// NewCueExperiments loads a cue configuration from filename and returns a list
// of ExperimentConfig structs.
// The configuration is validated against the embedded schema.
// One can specify the `base` config to overwrite its values from the Cue config.
// Leave `base` nil to construct a Cue config from scratch.
func NewCueExperiments(filename string, base *ExperimentConfig) ([]*ExperimentConfig, error) {
	var out []*ExperimentConfig
	for ec, err := range Experiments(filename, base) {
		if err != nil {
			return nil, err
		}
		out = append(out, ec)
	}
	return out, nil
}

// Experiments returns an iterator over experiment configurations that yields
// an ExperimentConfig and error for each experiment. It processes the cue file and
// validates each configuration against the schema. If [base] is non-nil, it is used
// to overwrite values in the Cue config, e.g., specified on the command line.
func Experiments(filename string, base *ExperimentConfig) iter.Seq2[*ExperimentConfig, error] {
	if base == nil {
		base = &ExperimentConfig{}
	}
	return func(yield func(*ExperimentConfig, error) bool) {
		ctx := cuecontext.New()
		configSchema := ctx.CompileString(schemaFile).LookupPath(cue.ParsePath("config"))
		if configSchema.Err() != nil {
			yield(nil, configSchema.Err())
			return
		}

		b, err := os.ReadFile(filename)
		if err != nil {
			yield(nil, err)
			return
		}
		elem := ctx.CompileString(string(b), cue.Filename(filename))
		if elem.Err() != nil {
			yield(nil, elem.Err())
			return
		}

		// check if elem contains an iterator that can yield a list of experiments
		it, err := cueIterator(elem)
		if err != nil {
			// if not an iterator, check if elem is a single config
			yield(decodeConfig(elem, configSchema, base))
			return
		}
		for it.Next() {
			config := it.Value()
			if !yield(decodeConfig(config, configSchema, base)) {
				return
			}
		}
	}
}

// cueIterator returns an iterator over a list of experiments, or
// one that expands into a list of experiments.
func cueIterator(elem cue.Value) (cue.Iterator, error) {
	it, e1 := elem.List()
	if e1 == nil {
		return it, nil
	}
	listVal := elem.LookupPath(cue.ParsePath("config.experiments"))
	if e2 := listVal.Err(); e2 != nil {
		return cue.Iterator{}, errors.Join(e1, e2)
	}
	return listVal.List()
}

// decodeConfig decodes a cue.Value into an ExperimentConfig, validating it against the schema.
func decodeConfig(elem, schema cue.Value, base *ExperimentConfig) (*ExperimentConfig, error) {
	config := elem.LookupPath(cue.ParsePath("config")) // config is a { config: { … } }
	if config.Err() != nil {
		return nil, fmt.Errorf("failed to get config from cue file: %w", config.Err())
	}
	unified := schema.Unify(config)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return nil, err
	}
	ec := base.Clone()
	if err := unified.Decode(ec); err != nil {
		return nil, err
	}
	return ec, nil
}
