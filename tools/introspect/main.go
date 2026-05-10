//go:build introspect

// Command introspect issues a GraphQL introspection query against the
// monday.com API and writes the result as an SDL file to schema/monday.graphql.
//
// Usage:
//
//	go run -tags=introspect ./tools/introspect
//
// The MONDAY_API_TOKEN environment variable must be set.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	endpoint    = "https://api.monday.com/v2"
	outFile     = "schema/monday.graphql"
	httpTimeout = 60 * time.Second
)

// introspectionQuery is the full introspection query per the GraphQL spec.
const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type { ...TypeRef }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}
`

// ---- JSON shapes for the introspection response ----

type introspectionResponse struct {
	Data struct {
		Schema introspSchema `json:"__schema"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type introspSchema struct {
	QueryType        *namedType         `json:"queryType"`
	MutationType     *namedType         `json:"mutationType"`
	SubscriptionType *namedType         `json:"subscriptionType"`
	Types            []introspType      `json:"types"`
	Directives       []introspDirective `json:"directives"`
}

type namedType struct {
	Name string `json:"name"`
}

type introspType struct {
	Kind          string             `json:"kind"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	Fields        []introspField     `json:"fields"`
	InputFields   []introspArg       `json:"inputFields"`
	Interfaces    []introspTypeRef   `json:"interfaces"`
	EnumValues    []introspEnumValue `json:"enumValues"`
	PossibleTypes []introspTypeRef   `json:"possibleTypes"`
}

type introspField struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Args              []introspArg   `json:"args"`
	Type              introspTypeRef `json:"type"`
	IsDeprecated      bool           `json:"isDeprecated"`
	DeprecationReason string         `json:"deprecationReason"`
}

type introspArg struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Type         introspTypeRef `json:"type"`
	DefaultValue *string        `json:"defaultValue"`
}

type introspTypeRef struct {
	Kind   string          `json:"kind"`
	Name   string          `json:"name"`
	OfType *introspTypeRef `json:"ofType"`
}

type introspEnumValue struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	IsDeprecated      bool   `json:"isDeprecated"`
	DeprecationReason string `json:"deprecationReason"`
}

type introspDirective struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Locations   []string     `json:"locations"`
	Args        []introspArg `json:"args"`
}

// ---- main ----

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "introspect: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	token := os.Getenv("MONDAY_API_TOKEN")
	if token == "" {
		return fmt.Errorf("MONDAY_API_TOKEN is not set; export it before running make schema")
	}

	fmt.Fprintln(os.Stderr, "introspect: fetching Monday.com GraphQL schema…")

	schema, err := fetchSchema(ctx, token)
	if err != nil {
		return fmt.Errorf("fetch schema: %w", err)
	}

	sdl := renderSDL(schema)

	// outFile is written relative to the current working directory.
	// make schema is always invoked from the repo root.
	outPath := outFile

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create schema dir: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(sdl), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}

	lines := strings.Count(sdl, "\n")
	fmt.Fprintf(os.Stderr, "introspect: wrote %s (%d lines)\n", outPath, lines)
	return nil
}

// fetchSchema issues the introspection query and returns the decoded __schema.
func fetchSchema(ctx context.Context, token string) (introspSchema, error) {
	body, err := json.Marshal(map[string]string{"query": introspectionQuery})
	if err != nil {
		return introspSchema{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return introspSchema{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	req.Header.Set("User-Agent", "mcli/introspect")
	req.Header.Set("API-Version", "2025-01")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return introspSchema{}, fmt.Errorf("HTTP POST: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return introspSchema{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	var result introspectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return introspSchema{}, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return introspSchema{}, fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}

	return result.Data.Schema, nil
}

// ---- SDL renderer ----

// renderSDL converts an introspection __schema to GraphQL SDL text.
// Built-in types (starting with "__") are omitted.
// Types are sorted alphabetically for stable output.
func renderSDL(s introspSchema) string {
	var buf strings.Builder

	buf.WriteString("# Monday.com GraphQL Schema\n")
	buf.WriteString("# Generated by tools/introspect — do not edit by hand.\n")
	buf.WriteString("# Refresh: make schema\n\n")

	// Schema declaration when non-default root names are used.
	hasNonDefault := (s.QueryType != nil && s.QueryType.Name != "Query") ||
		(s.MutationType != nil && s.MutationType.Name != "Mutation") ||
		(s.SubscriptionType != nil && s.SubscriptionType.Name != "Subscription")
	if hasNonDefault {
		buf.WriteString("schema {\n")
		if s.QueryType != nil {
			fmt.Fprintf(&buf, "  query: %s\n", s.QueryType.Name)
		}
		if s.MutationType != nil {
			fmt.Fprintf(&buf, "  mutation: %s\n", s.MutationType.Name)
		}
		if s.SubscriptionType != nil {
			fmt.Fprintf(&buf, "  subscription: %s\n", s.SubscriptionType.Name)
		}
		buf.WriteString("}\n\n")
	}

	// Sort types for stable output.
	types := slices.Clone(s.Types)
	slices.SortFunc(types, func(a, b introspType) int {
		return cmp.Compare(a.Name, b.Name)
	})

	for _, t := range types {
		if isBuiltin(t.Name) {
			continue
		}
		switch t.Kind {
		case "SCALAR":
			renderScalar(&buf, t)
		case "ENUM":
			renderEnum(&buf, t)
		case "OBJECT":
			renderObject(&buf, t)
		case "INTERFACE":
			renderInterface(&buf, t)
		case "UNION":
			renderUnion(&buf, t)
		case "INPUT_OBJECT":
			renderInputObject(&buf, t)
		}
	}

	// Directives.
	directives := slices.Clone(s.Directives)
	slices.SortFunc(directives, func(a, b introspDirective) int {
		return cmp.Compare(a.Name, b.Name)
	})
	for _, d := range directives {
		if isBuiltinDirective(d.Name) {
			continue
		}
		renderDirective(&buf, d)
	}

	return buf.String()
}

func isBuiltin(name string) bool {
	return strings.HasPrefix(name, "__") ||
		name == "String" || name == "Boolean" || name == "Int" ||
		name == "Float" || name == "ID"
}

func isBuiltinDirective(name string) bool {
	switch name {
	case "skip", "include", "deprecated", "specifiedBy":
		return true
	}
	return false
}

// ---- per-kind renderers ----

func renderDescription(buf *strings.Builder, desc string, indent string) {
	if desc == "" {
		return
	}
	lines := strings.Split(desc, "\n")
	if len(lines) == 1 {
		fmt.Fprintf(buf, "%s\"\"\"%s\"\"\"\n", indent, escapeDQ(desc))
		return
	}
	fmt.Fprintf(buf, "%s\"\"\"\n", indent)
	for _, l := range lines {
		if l == "" {
			buf.WriteString(indent + "\n")
		} else {
			fmt.Fprintf(buf, "%s%s\n", indent, l)
		}
	}
	fmt.Fprintf(buf, "%s\"\"\"\n", indent)
}

func escapeDQ(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func renderScalar(buf *strings.Builder, t introspType) {
	renderDescription(buf, t.Description, "")
	fmt.Fprintf(buf, "scalar %s\n\n", t.Name)
}

func renderEnum(buf *strings.Builder, t introspType) {
	renderDescription(buf, t.Description, "")
	fmt.Fprintf(buf, "enum %s {\n", t.Name)
	for _, v := range t.EnumValues {
		renderDescription(buf, v.Description, "  ")
		if v.IsDeprecated && v.DeprecationReason != "" {
			fmt.Fprintf(buf, "  %s @deprecated(reason: %q)\n", v.Name, v.DeprecationReason)
		} else if v.IsDeprecated {
			fmt.Fprintf(buf, "  %s @deprecated\n", v.Name)
		} else {
			fmt.Fprintf(buf, "  %s\n", v.Name)
		}
	}
	buf.WriteString("}\n\n")
}

func renderObject(buf *strings.Builder, t introspType) {
	renderDescription(buf, t.Description, "")
	fmt.Fprintf(buf, "type %s", t.Name)
	if len(t.Interfaces) > 0 {
		names := make([]string, len(t.Interfaces))
		for i, iface := range t.Interfaces {
			names[i] = iface.Name
		}
		fmt.Fprintf(buf, " implements %s", strings.Join(names, " & "))
	}
	buf.WriteString(" {\n")
	renderFields(buf, t.Fields)
	buf.WriteString("}\n\n")
}

func renderInterface(buf *strings.Builder, t introspType) {
	renderDescription(buf, t.Description, "")
	fmt.Fprintf(buf, "interface %s {\n", t.Name)
	renderFields(buf, t.Fields)
	buf.WriteString("}\n\n")
}

func renderUnion(buf *strings.Builder, t introspType) {
	renderDescription(buf, t.Description, "")
	if len(t.PossibleTypes) == 0 {
		return
	}
	names := make([]string, len(t.PossibleTypes))
	for i, pt := range t.PossibleTypes {
		names[i] = pt.Name
	}
	fmt.Fprintf(buf, "union %s = %s\n\n", t.Name, strings.Join(names, " | "))
}

func renderInputObject(buf *strings.Builder, t introspType) {
	renderDescription(buf, t.Description, "")
	fmt.Fprintf(buf, "input %s {\n", t.Name)
	for _, f := range t.InputFields {
		renderDescription(buf, f.Description, "  ")
		if f.DefaultValue != nil {
			fmt.Fprintf(buf, "  %s: %s = %s\n", f.Name, typeRefString(f.Type), *f.DefaultValue)
		} else {
			fmt.Fprintf(buf, "  %s: %s\n", f.Name, typeRefString(f.Type))
		}
	}
	buf.WriteString("}\n\n")
}

func renderFields(buf *strings.Builder, fields []introspField) {
	for _, f := range fields {
		renderDescription(buf, f.Description, "  ")
		if len(f.Args) == 0 {
			if f.IsDeprecated && f.DeprecationReason != "" {
				fmt.Fprintf(buf, "  %s: %s @deprecated(reason: %q)\n", f.Name, typeRefString(f.Type), f.DeprecationReason)
			} else if f.IsDeprecated {
				fmt.Fprintf(buf, "  %s: %s @deprecated\n", f.Name, typeRefString(f.Type))
			} else {
				fmt.Fprintf(buf, "  %s: %s\n", f.Name, typeRefString(f.Type))
			}
		} else {
			// Multi-line args when > 1.
			if len(f.Args) == 1 {
				a := f.Args[0]
				argStr := formatArg(a)
				if f.IsDeprecated && f.DeprecationReason != "" {
					fmt.Fprintf(buf, "  %s(%s): %s @deprecated(reason: %q)\n", f.Name, argStr, typeRefString(f.Type), f.DeprecationReason)
				} else if f.IsDeprecated {
					fmt.Fprintf(buf, "  %s(%s): %s @deprecated\n", f.Name, argStr, typeRefString(f.Type))
				} else {
					fmt.Fprintf(buf, "  %s(%s): %s\n", f.Name, argStr, typeRefString(f.Type))
				}
			} else {
				fmt.Fprintf(buf, "  %s(\n", f.Name)
				for _, a := range f.Args {
					renderDescription(buf, a.Description, "    ")
					fmt.Fprintf(buf, "    %s\n", formatArg(a))
				}
				if f.IsDeprecated && f.DeprecationReason != "" {
					fmt.Fprintf(buf, "  ): %s @deprecated(reason: %q)\n", typeRefString(f.Type), f.DeprecationReason)
				} else if f.IsDeprecated {
					fmt.Fprintf(buf, "  ): %s @deprecated\n", typeRefString(f.Type))
				} else {
					fmt.Fprintf(buf, "  ): %s\n", typeRefString(f.Type))
				}
			}
		}
	}
}

func formatArg(a introspArg) string {
	if a.DefaultValue != nil {
		return fmt.Sprintf("%s: %s = %s", a.Name, typeRefString(a.Type), *a.DefaultValue)
	}
	return fmt.Sprintf("%s: %s", a.Name, typeRefString(a.Type))
}

func renderDirective(buf *strings.Builder, d introspDirective) {
	renderDescription(buf, d.Description, "")
	fmt.Fprintf(buf, "directive @%s", d.Name)
	if len(d.Args) > 0 {
		if len(d.Args) == 1 {
			fmt.Fprintf(buf, "(%s)", formatArg(d.Args[0]))
		} else {
			buf.WriteString("(\n")
			for _, a := range d.Args {
				renderDescription(buf, a.Description, "  ")
				fmt.Fprintf(buf, "  %s\n", formatArg(a))
			}
			buf.WriteString(")")
		}
	}
	if len(d.Locations) > 0 {
		fmt.Fprintf(buf, " on %s", strings.Join(d.Locations, " | "))
	}
	buf.WriteString("\n\n")
}

// typeRefString converts a nested TypeRef to a GraphQL type string,
// e.g. NON_NULL(LIST(NON_NULL(String))) → "[String!]!".
func typeRefString(t introspTypeRef) string {
	switch t.Kind {
	case "NON_NULL":
		if t.OfType != nil {
			return typeRefString(*t.OfType) + "!"
		}
		return "!"
	case "LIST":
		if t.OfType != nil {
			return "[" + typeRefString(*t.OfType) + "]"
		}
		return "[]"
	default:
		return t.Name
	}
}
