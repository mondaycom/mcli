package apischema

import (
	"slices"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// preferredScalars are included first (in order) when they exist on a type.
var preferredScalars = []string{"id", "name", "state", "kind"}

// DefaultSelection returns a GraphQL selection set string like "{ id name }"
// for the given type, up to depth levels deep. depth 0 or unknown type returns
// "{ id }" as a safe fallback.
func DefaultSelection(typeName string, s *ast.Schema, depth int) string {
	if depth <= 0 || s == nil {
		return "{ id }"
	}
	def := s.Types[typeName]
	if def == nil {
		return "{ id }"
	}
	// Scalar, enum, and input types have no selection set in GraphQL.
	switch def.Kind {
	case ast.Scalar, ast.Enum, ast.InputObject:
		return ""
	}
	if def.Kind != ast.Object && def.Kind != ast.Interface {
		return "{ id }"
	}
	visited := map[string]bool{}
	return buildSelection(def, s, depth, visited)
}

func buildSelection(def *ast.Definition, s *ast.Schema, depth int, visited map[string]bool) string {
	if def == nil || depth <= 0 || visited[def.Name] {
		return ""
	}
	visited[def.Name] = true
	defer func() { visited[def.Name] = false }()

	var parts []string

	// Collect scalar/enum fields: preferred first, then alphabetical rest.
	preferred := make([]string, 0, 4)
	others := make([]string, 0, len(def.Fields))

	for _, f := range def.Fields {
		leaf := leafTypeName(f.Type)
		td := s.Types[leaf]
		isScalarOrEnum := td == nil || td.Kind == ast.Scalar || td.Kind == ast.Enum
		if isScalarOrEnum {
			if isPreferred(f.Name) {
				preferred = append(preferred, f.Name)
			} else {
				others = append(others, f.Name)
			}
		}
	}

	// Sort preferred by canonical order.
	sort.SliceStable(preferred, func(i, j int) bool {
		return preferredIndex(preferred[i]) < preferredIndex(preferred[j])
	})
	sort.Strings(others)

	parts = append(parts, preferred...)
	parts = append(parts, others...)

	if depth > 1 {
		// Add object/interface fields recursively.
		for _, f := range def.Fields {
			leaf := leafTypeName(f.Type)
			td := s.Types[leaf]
			if td == nil || (td.Kind != ast.Object && td.Kind != ast.Interface) {
				continue
			}
			if visited[td.Name] {
				continue
			}
			sub := buildSelection(td, s, depth-1, visited)
			if sub != "" {
				parts = append(parts, f.Name+" "+sub)
			}
		}
	}

	if len(parts) == 0 {
		return "{ id }"
	}
	return "{ " + strings.Join(parts, " ") + " }"
}

func isPreferred(name string) bool {
	return slices.Contains(preferredScalars, name)
}

func preferredIndex(name string) int {
	for i, p := range preferredScalars {
		if p == name {
			return i
		}
	}
	return len(preferredScalars)
}
