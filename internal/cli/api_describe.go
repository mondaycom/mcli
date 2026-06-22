package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/vektah/gqlparser/v2/ast"
)

func newAPIDescribeCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "describe <name>",
		Short: "Describe an API operation or type",
		Long: `Describe a monday.com GraphQL operation or type.

For operations: shows type (query/mutation), description, arguments, and return type.
For types: shows all fields with their types and descriptions.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPIDescribe(cmd, args[0], jsonOut)
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	return cmd
}

// apiDescribeOpOutput is the JSON shape for an operation description.
type apiDescribeOpOutput struct {
	Name              string           `json:"name"`
	Type              string           `json:"type"`
	Description       string           `json:"description"`
	ReturnType        string           `json:"return_type"`
	BuiltinEquivalent string           `json:"builtin_equivalent,omitempty"`
	Args              []apiDescribeArg `json:"args"`
}

type apiDescribeArg struct {
	Name         string `json:"name"`
	TypeString   string `json:"type"`
	IsRequired   bool   `json:"required"`
	IsJSON       bool   `json:"is_json,omitempty"`
	Description  string `json:"description,omitempty"`
	DefaultValue string `json:"default,omitempty"`
}

// apiDescribeTypeOutput is the JSON shape for a type description.
type apiDescribeTypeOutput struct {
	Name        string             `json:"name"`
	Kind        string             `json:"kind"`
	Description string             `json:"description,omitempty"`
	Fields      []apiDescribeField `json:"fields,omitempty"`
	EnumValues  []string           `json:"enum_values,omitempty"`
}

type apiDescribeField struct {
	Name        string `json:"name"`
	TypeString  string `json:"type"`
	Description string `json:"description,omitempty"`
}

func runAPIDescribe(cmd *cobra.Command, name string, jsonOut bool) error {
	// Try as operation first.
	queryFields, err := apischema.QueryFields()
	if err != nil {
		return errs.Internal("load query fields: %v", err)
	}
	mutFields, err := apischema.MutationFields()
	if err != nil {
		return errs.Internal("load mutation fields: %v", err)
	}

	for _, f := range queryFields {
		if f.Name == name {
			return printOpDescription(cmd, f, jsonOut)
		}
	}
	for _, f := range mutFields {
		if f.Name == name {
			return printOpDescription(cmd, f, jsonOut)
		}
	}

	// Try as type.
	def, err := apischema.TypeDef(name)
	if err != nil {
		return errs.Usage("%q is not a known operation or type — run 'mcli api list' to browse operations", name)
	}
	return printTypeDescription(cmd, def, jsonOut)
}

func printOpDescription(cmd *cobra.Command, f apischema.FieldDef, jsonOut bool) error {
	opType := "query"
	if !f.IsQuery {
		opType = "mutation"
	}
	eq := builtinEquivalents[f.Name]

	if jsonOut || globals.JSON {
		args := make([]apiDescribeArg, len(f.Args))
		for i, a := range f.Args {
			args[i] = apiDescribeArg{
				Name:         a.Name,
				TypeString:   a.TypeString,
				IsRequired:   a.IsRequired,
				IsJSON:       a.IsJSON,
				Description:  a.Description,
				DefaultValue: a.DefaultValue,
			}
		}
		out := apiDescribeOpOutput{
			Name:              f.Name,
			Type:              opType,
			Description:       f.Description,
			ReturnType:        f.ReturnType,
			BuiltinEquivalent: eq,
			Args:              args,
		}
		data, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	o := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(o, "Name:        %s\n", f.Name)
	_, _ = fmt.Fprintf(o, "Type:        %s\n", opType)
	_, _ = fmt.Fprintf(o, "Returns:     %s\n", f.ReturnType)
	if eq != "" {
		_, _ = fmt.Fprintf(o, "See also:    %s\n", eq)
	}
	if f.Description != "" {
		_, _ = fmt.Fprintf(o, "Description: %s\n", f.Description)
	}

	if len(f.Args) > 0 {
		_, _ = fmt.Fprintln(o, "\nArguments:")
		tw := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  NAME\tTYPE\tREQUIRED\tDESCRIPTION")
		for _, a := range f.Args {
			req := ""
			if a.IsRequired {
				req = "yes"
			}
			desc := truncate(a.Description, 60)
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", a.Name, a.TypeString, req, desc)
		}
		_ = tw.Flush()
	}

	return nil
}

func printTypeDescription(cmd *cobra.Command, def *ast.Definition, jsonOut bool) error {
	if jsonOut || globals.JSON {
		out := apiDescribeTypeOutput{
			Name:        def.Name,
			Kind:        string(def.Kind),
			Description: def.Description,
		}
		for _, f := range def.Fields {
			out.Fields = append(out.Fields, apiDescribeField{
				Name:        f.Name,
				TypeString:  f.Type.String(),
				Description: f.Description,
			})
		}
		for _, ev := range def.EnumValues {
			out.EnumValues = append(out.EnumValues, ev.Name)
		}
		data, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	o := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(o, "Name: %s\n", def.Name)
	_, _ = fmt.Fprintf(o, "Kind: %s\n", def.Kind)
	if def.Description != "" {
		_, _ = fmt.Fprintf(o, "Description: %s\n", def.Description)
	}

	if len(def.Fields) > 0 {
		_, _ = fmt.Fprintln(o, "\nFields:")
		tw := tabwriter.NewWriter(o, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  NAME\tTYPE\tDESCRIPTION")
		for _, f := range def.Fields {
			desc := truncate(f.Description, 60)
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", f.Name, f.Type.String(), desc)
		}
		_ = tw.Flush()
	}

	if len(def.EnumValues) > 0 {
		_, _ = fmt.Fprintln(o, "\nValues:")
		for _, ev := range def.EnumValues {
			_, _ = fmt.Fprintf(o, "  %s\n", ev.Name)
		}
	}

	return nil
}
