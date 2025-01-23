package config

import (
	_ "embed"
	"os"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

//go:embed schema.cue
var schemaFile string

// Load loads a cue configuration from filename and returns a Config struct.
// The configuration is validated against the schema embedded in the binary.
func Load(filename string) (*HostConfig, error) {
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
	conf := &HostConfig{}
	if err := cfg.Decode(conf); err != nil {
		return nil, err
	}
	return conf, nil
}
