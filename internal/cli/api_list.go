package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/errs"
)

// apiListItem is the JSON shape for one entry in 'mcli api list --json'.
type apiListItem struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	Type              string `json:"type"`
	BuiltinEquivalent string `json:"builtin_equivalent,omitempty"`
}

func newAPIListCmd() *cobra.Command {
	var (
		typeFilter string
		noBuiltins bool
		jsonOut    bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available API operations",
		Long:  "List all monday.com GraphQL query and mutation operations available via 'mcli api'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAPIList(cmd, typeFilter, noBuiltins, jsonOut)
		},
	}

	cmd.Flags().StringVar(&typeFilter, "type", "", "filter by type: query or mutation")
	cmd.Flags().BoolVar(&noBuiltins, "no-builtins", false, "hide operations that have a built-in mcli equivalent")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON array")

	return cmd
}

func runAPIList(cmd *cobra.Command, typeFilter string, noBuiltins bool, jsonOut bool) error {
	if typeFilter != "" && typeFilter != "query" && typeFilter != "mutation" {
		return errs.Usage("--type must be 'query' or 'mutation', got %q", typeFilter)
	}

	var items []apiListItem

	if typeFilter != "mutation" {
		fields, err := apischema.QueryFields()
		if err != nil {
			return errs.Internal("load query fields: %v", err)
		}
		for _, f := range fields {
			eq := builtinEquivalents[f.Name]
			if noBuiltins && eq != "" {
				continue
			}
			items = append(items, apiListItem{
				Name:              f.Name,
				Description:       f.Description,
				Type:              "query",
				BuiltinEquivalent: eq,
			})
		}
	}

	if typeFilter != "query" {
		fields, err := apischema.MutationFields()
		if err != nil {
			return errs.Internal("load mutation fields: %v", err)
		}
		for _, f := range fields {
			eq := builtinEquivalents[f.Name]
			if noBuiltins && eq != "" {
				continue
			}
			items = append(items, apiListItem{
				Name:              f.Name,
				Description:       f.Description,
				Type:              "mutation",
				BuiltinEquivalent: eq,
			})
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })

	if jsonOut || globals.JSON {
		data, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, item := range items {
		desc := truncate(item.Description, 55)
		extra := ""
		if item.BuiltinEquivalent != "" {
			extra = "[see: " + item.BuiltinEquivalent + "]"
		}
		_, _ = fmt.Fprintf(w, "%-40s %-60s %s\n", item.Name, desc, extra)
	}
	return w.Flush()
}

func truncate(s string, max int) string {
	// Collapse whitespace / newlines in descriptions.
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
