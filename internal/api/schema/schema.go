// Package apischema parses the monday.com GraphQL SDL and exposes query/mutation
// field definitions for dynamic command generation.
package apischema

import (
	"fmt"
	"sync"

	gqlparser "github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"

	rawschema "github.com/mondaycom/mcli/schema"
)

// FieldDef describes a single top-level query or mutation field.
type FieldDef struct {
	Name        string
	Description string
	Args        []ArgDef
	ReturnType  string
	IsQuery     bool
}

// ArgDef describes one argument on a FieldDef.
type ArgDef struct {
	Name         string
	Description  string
	TypeString   string
	IsRequired   bool
	IsJSON       bool
	DefaultValue string
}

var (
	loadOnce     sync.Once
	loadedSchema *ast.Schema
	loadErr      error
)

// Load parses the embedded SDL once and caches the result.
func Load() (*ast.Schema, error) {
	loadOnce.Do(func() {
		src := &ast.Source{Name: "monday.graphql", Input: rawschema.SDL}
		s, err := gqlparser.LoadSchema(src)
		if err != nil {
			loadErr = fmt.Errorf("parse schema: %w", err)
			return
		}
		loadedSchema = s
	})
	return loadedSchema, loadErr
}

// QueryFields returns FieldDef for each field on the Query type.
func QueryFields() ([]FieldDef, error) {
	s, err := Load()
	if err != nil {
		return nil, err
	}
	return fieldsForType(s.Query, true), nil
}

// MutationFields returns FieldDef for each field on the Mutation type.
func MutationFields() ([]FieldDef, error) {
	s, err := Load()
	if err != nil {
		return nil, err
	}
	return fieldsForType(s.Mutation, false), nil
}

// TypeDef returns the ast.Definition for the named type.
func TypeDef(name string) (*ast.Definition, error) {
	s, err := Load()
	if err != nil {
		return nil, err
	}
	def := s.Types[name]
	if def == nil {
		return nil, fmt.Errorf("type %q not found in schema", name)
	}
	return def, nil
}

func fieldsForType(def *ast.Definition, isQuery bool) []FieldDef {
	if def == nil {
		return nil
	}
	out := make([]FieldDef, 0, len(def.Fields))
	for _, f := range def.Fields {
		fd := FieldDef{
			Name:        f.Name,
			Description: f.Description,
			ReturnType:  f.Type.String(),
			IsQuery:     isQuery,
			Args:        make([]ArgDef, 0, len(f.Arguments)),
		}
		for _, arg := range f.Arguments {
			ad := ArgDef{
				Name:        arg.Name,
				Description: arg.Description,
				TypeString:  arg.Type.String(),
				IsRequired:  arg.Type.NonNull,
				IsJSON:      leafTypeName(arg.Type) == "JSON",
			}
			if arg.DefaultValue != nil {
				ad.DefaultValue = arg.DefaultValue.String()
			}
			fd.Args = append(fd.Args, ad)
		}
		out = append(out, fd)
	}
	return out
}

// leafTypeName unwraps NonNull and List wrappers to return the named type.
func leafTypeName(t *ast.Type) string {
	if t == nil {
		return ""
	}
	if t.NamedType != "" {
		return t.NamedType
	}
	return leafTypeName(t.Elem)
}
