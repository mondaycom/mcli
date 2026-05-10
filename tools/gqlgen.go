//go:build tools

// Package tools pins tooling dependencies so that go mod tidy retains them
// and go generate can resolve the genqlient binary.
//
// Run from the repo root:
//
//	go generate ./...
package tools

import _ "github.com/Khan/genqlient/generate"
