// Package apischema parses the monday.com GraphQL SDL and exposes query/mutation
// field definitions for dynamic command generation.
package apischema

import (
	"fmt"
	"os"
	"path/filepath"
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
	mu           sync.Mutex
	cachedSchema *ast.Schema
	cacheErr     error
	cacheDir     string
	cacheLoaded  bool
)

// SetConfigDir sets the directory used to locate a cached schema.graphql file.
// It resets the cache so the next Load() call re-resolves from the new directory.
// Call with an empty string to revert to the embedded fallback.
func SetConfigDir(dir string) {
	mu.Lock()
	defer mu.Unlock()
	cacheDir = dir
	cacheLoaded = false
	cachedSchema = nil
	cacheErr = nil
}

// CachedSchemaPath returns the path to the locally cached schema file.
func CachedSchemaPath(configDir string) string {
	return filepath.Join(configDir, "schema.graphql")
}

// Load parses the GraphQL schema and caches the result.
// If a cacheDir is set and a schema.graphql file exists there, it is used.
// On any read or parse error it falls back to the embedded SDL.
func Load() (*ast.Schema, error) {
	mu.Lock()
	defer mu.Unlock()

	if cacheLoaded {
		return cachedSchema, cacheErr
	}

	var s *ast.Schema
	var err error

	if cacheDir != "" {
		data, readErr := os.ReadFile(filepath.Join(cacheDir, "schema.graphql"))
		if readErr == nil {
			src := &ast.Source{Name: "schema.graphql", Input: string(data)}
			s, err = gqlparser.LoadSchema(src)
			if err != nil {
				err = fmt.Errorf("parse local schema: %w", err)
			}
		}
	}

	if s == nil {
		src := &ast.Source{Name: "monday.graphql", Input: rawschema.SDL}
		s, err = gqlparser.LoadSchema(src)
		if err != nil {
			err = fmt.Errorf("parse schema: %w", err)
		}
	}

	cachedSchema = s
	cacheErr = err
	cacheLoaded = true

	return cachedSchema, cacheErr
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
