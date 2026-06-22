package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vektah/gqlparser/v2/ast"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/errs"
)

// builtinEquivalents maps monday.com operation names to the mcli built-in
// command that already covers that operation. Used by 'api list' and
// 'api describe' to surface alternatives.
var builtinEquivalents = map[string]string{
	"boards":              "mcli board list",
	"board":               "mcli board get",
	"create_board":        "mcli board create",
	"delete_board":        "mcli board delete",
	"archive_board":       "mcli board archive",
	"items_page":          "mcli item list",
	"create_item":         "mcli item create",
	"delete_item":         "mcli item delete",
	"create_webhook":      "mcli webhook create",
	"delete_webhook":      "mcli webhook delete",
	"get_webhooks":        "mcli webhook list",
	"create_notification": "mcli notification list",
	"me":                  "mcli me",
	"workspaces":          "mcli workspace list",
	"folders":             "mcli folder list",
}

// newAPICmd returns the 'mcli api' command with dynamic dispatch.
func newAPICmd() *cobra.Command {
	var (
		argFlags []string
		argFile  string
		selectFL string
		depth    int
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "api <operation> [flags]",
		Short: "Execute any monday.com API operation dynamically",
		Long: `Execute any monday.com GraphQL query or mutation by name.

Use 'mcli api list' to browse all ~250 available operations.
Use 'mcli api describe <operation>' to inspect arguments and return type.

Example:
  mcli api me
  mcli api change_column_value --arg board_id=123 --arg item_id=456 \
    --arg column_id=status --arg value='{"label":"Done"}'`,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errs.Usage("provide an operation name, or use 'mcli api list'")
			}
			return runAPIDynamic(cmd, args[0], argFlags, argFile, selectFL, depth, dryRun)
		},
	}

	cmd.Flags().StringArrayVar(&argFlags, "arg", nil, "argument: key=value (repeatable, auto-typed)")
	cmd.Flags().StringVar(&argFile, "arg-file", "", "JSON file with argument values")
	cmd.Flags().StringVar(&selectFL, "select", "", "override selection set: comma-separated fields, e.g. id,name,column_values{id,text}")
	cmd.Flags().IntVar(&depth, "depth", 2, "auto-selection depth for return type")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print generated GraphQL without executing")

	cmd.AddCommand(newAPIListCmd())
	cmd.AddCommand(newAPIDescribeCmd())

	return cmd
}

func runAPIDynamic(cmd *cobra.Command, opName string, argFlags []string, argFile, selectFL string, depth int, dryRun bool) error {
	s, err := apischema.Load()
	if err != nil {
		return errs.Internal("load schema: %v", err)
	}

	// Look up operation in Query then Mutation.
	queryFields, err := apischema.QueryFields()
	if err != nil {
		return errs.Internal("load query fields: %v", err)
	}
	mutFields, err := apischema.MutationFields()
	if err != nil {
		return errs.Internal("load mutation fields: %v", err)
	}

	var field *apischema.FieldDef
	for i := range queryFields {
		if queryFields[i].Name == opName {
			f := queryFields[i]
			field = &f
			break
		}
	}
	if field == nil {
		for i := range mutFields {
			if mutFields[i].Name == opName {
				f := mutFields[i]
				field = &f
				break
			}
		}
	}
	if field == nil {
		return errs.Usage("unknown operation %q — run 'mcli api list' to see available operations", opName)
	}

	// Parse --arg flags.
	vars, err := parseVarFlags(argFlags)
	if err != nil {
		return err
	}
	if argFile != "" {
		if err := mergeVarsFile(vars, argFile); err != nil {
			return err
		}
	}

	// Validate required args are all present.
	var missing []string
	for _, ad := range field.Args {
		if ad.IsRequired && ad.DefaultValue == "" {
			if _, ok := vars[ad.Name]; !ok {
				missing = append(missing, ad.Name)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return errs.Usage("operation %q requires: %s", opName, strings.Join(missing, ", "))
	}

	// Coerce JSON scalar args.
	apischema.CoerceArgs(field.Args, vars)

	// Build selection set.
	sel := buildSelectionStr(selectFL, field.ReturnType, s, depth)

	// Build GraphQL operation string.
	gqlStr := buildGQLOperation(field, vars, sel)

	if dryRun {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), gqlStr)
		return err
	}

	return executeRawQuery(cmd, gqlStr, vars)
}

// buildSelectionStr resolves the selection set from --select or auto-generation.
func buildSelectionStr(selectFL, returnType string, s *ast.Schema, depth int) string {
	if selectFL != "" {
		return convertSelectFlag(selectFL)
	}
	base := unwrapTypeName(returnType)
	return apischema.DefaultSelection(base, s, depth)
}

// convertSelectFlag converts a comma-notation like "id,name,col{id,text}"
// to a GraphQL selection string "{ id name col { id text } }".
func convertSelectFlag(s string) string {
	return "{ " + parseSelectSegments(s) + " }"
}

// parseSelectSegments parses the comma-separated field list with optional
// nested braces like "id,name,col{id,text}".
func parseSelectSegments(s string) string {
	var parts []string
	i := 0
	for i < len(s) {
		// Find next comma or opening brace.
		j := i
		for j < len(s) && s[j] != ',' && s[j] != '{' {
			j++
		}
		name := strings.TrimSpace(s[i:j])
		if j < len(s) && s[j] == '{' {
			// Find matching closing brace.
			depth := 1
			k := j + 1
			for k < len(s) && depth > 0 {
				switch s[k] {
				case '{':
					depth++
				case '}':
					depth--
				}
				k++
			}
			inner := s[j+1 : k-1]
			if name != "" {
				parts = append(parts, name+" { "+parseSelectSegments(inner)+" }")
			}
			i = k
			if i < len(s) && s[i] == ',' {
				i++
			}
		} else {
			if name != "" {
				parts = append(parts, name)
			}
			i = j + 1
		}
	}
	return strings.Join(parts, " ")
}

// unwrapTypeName strips List and NonNull wrappers from a type string like
// "[Board!]!" returning "Board".
func unwrapTypeName(t string) string {
	s := strings.TrimSuffix(t, "!")
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSuffix(s, "!")
	return strings.TrimSpace(s)
}

// buildGQLOperation constructs the full GraphQL operation string.
func buildGQLOperation(field *apischema.FieldDef, vars map[string]any, sel string) string {
	opKind := "query"
	if !field.IsQuery {
		opKind = "mutation"
	}

	// Build variable declarations for args that are required or provided.
	var varDecls []string
	var fieldArgs []string
	for _, ad := range field.Args {
		_, provided := vars[ad.Name]
		if !ad.IsRequired && !provided {
			continue
		}
		varDecls = append(varDecls, fmt.Sprintf("$%s: %s", ad.Name, ad.TypeString))
		fieldArgs = append(fieldArgs, fmt.Sprintf("%s: $%s", ad.Name, ad.Name))
	}

	var sb strings.Builder
	sb.WriteString(opKind)
	if len(varDecls) > 0 {
		sb.WriteString("(")
		sb.WriteString(strings.Join(varDecls, ", "))
		sb.WriteString(")")
	}
	sb.WriteString(" { ")
	sb.WriteString(field.Name)
	if len(fieldArgs) > 0 {
		sb.WriteString("(")
		sb.WriteString(strings.Join(fieldArgs, ", "))
		sb.WriteString(")")
	}
	if sel != "" {
		sb.WriteString(" ")
		sb.WriteString(sel)
	}
	sb.WriteString(" }")
	return sb.String()
}
