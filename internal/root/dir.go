// Package root provides the root directory of the project.
package root

import (
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)

	// Dir is the root directory of the project.
	Dir = filepath.Join(filepath.Dir(b), "../..")
)
