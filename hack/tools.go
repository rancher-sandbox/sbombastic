//go:build tools
// +build tools

// This project uses `go run` instead of the `tools.go` pattern to pin tool versions in the Makefile.
//
//	However, we need to import `k8s.io/code-generator` as it is required by the build scripts `update-codegen.sh` and `verify-codegen.sh`.
package tools

import (
	_ "k8s.io/code-generator"
)
