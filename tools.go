//go:build tools

package hotstuff

import (
	_ "cuelang.org/go/cmd/cue"
	_ "github.com/relab/gorums/cmd/protoc-gen-gorums"
	_ "go.uber.org/mock/mockgen"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
