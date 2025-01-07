//go:build tools

package hotstuff

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/relab/gorums/cmd/protoc-gen-gorums"
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
)
