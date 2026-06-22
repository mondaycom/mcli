// Package schema exposes the monday.com GraphQL SDL as an embedded string.
package schema

import _ "embed"

//go:embed monday.graphql
var SDL string
