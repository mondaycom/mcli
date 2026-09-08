// Package schema exposes the monday.com GraphQL SDL as an embedded string.
package schema

import (
	_ "embed"
	"strings"
	"time"
)

//go:embed monday.graphql
var SDL string

// fetchedAtRaw is the date the embedded SDL was introspected, in YYYY-MM-DD form.
// It is rewritten by `make schema` (see tools/introspect).
//
//go:embed fetched_at.txt
var fetchedAtRaw string

// fetchedAtLayout is the date format stored in fetched_at.txt.
const fetchedAtLayout = "2006-01-02"

// FetchedAt reports the date the embedded SDL was introspected from the monday
// API. It returns the zero Time if the recorded date is missing or unparseable,
// so callers must check IsZero before treating the value as an age.
func FetchedAt() time.Time {
	t, err := time.Parse(fetchedAtLayout, strings.TrimSpace(fetchedAtRaw))
	if err != nil {
		return time.Time{}
	}
	return t
}
