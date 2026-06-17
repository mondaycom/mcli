package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/errs"
)

const (
	queriesSubDir = "queries"
	queryExt      = ".graphql"
)

// localQueriesDir returns the path to the local query store directory
// (.mcli/queries/ relative to the working directory).
func localQueriesDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Join(wd, ".mcli", queriesSubDir), nil
}

// globalQueriesDir returns the path to the global query store directory
// (~/.config/mcli/queries/).
func globalQueriesDir() string {
	return filepath.Join(resolveConfigDir(), queriesSubDir)
}

// savedQueryPath returns the full file path for a named query in the given directory.
func savedQueryPath(dir, name string) string {
	return filepath.Join(dir, name+queryExt)
}

// saveSavedQuery writes queryStr to the appropriate location.
// If global is true the query is written to the global dir; otherwise local.
func saveSavedQuery(name, queryStr string, global bool) error {
	var dir string
	if global {
		dir = globalQueriesDir()
	} else {
		localDir, err := localQueriesDir()
		if err != nil {
			return err
		}
		dir = localDir
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create queries dir %s: %w", dir, err)
	}

	path := savedQueryPath(dir, name)
	if err := os.WriteFile(path, []byte(queryStr), 0o600); err != nil {
		return fmt.Errorf("write query %s: %w", path, err)
	}
	return nil
}

// loadSavedQuery finds a query by name, checking local first then global.
// Returns errs.NotFound when no matching query exists in either location.
func loadSavedQuery(name string) (string, error) {
	localDir, err := localQueriesDir()
	if err != nil {
		return "", err
	}

	// Try local first.
	localPath := savedQueryPath(localDir, name)
	if data, err := os.ReadFile(localPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	// Then global.
	globalPath := savedQueryPath(globalQueriesDir(), name)
	if data, err := os.ReadFile(globalPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	return "", errs.NotFound("saved query %q not found (checked local and global)", name)
}

// savedQueryEntry describes a single saved query returned by listSavedQueries.
type savedQueryEntry struct {
	Name   string `json:"name"`
	Source string `json:"source"` // "local" or "global"
}

// listSavedQueries returns all saved queries from both local and global stores.
// Local entries come first; within each set, entries are in filesystem order.
// If a name exists in both stores, both are included with their respective source.
func listSavedQueries() ([]savedQueryEntry, error) {
	var entries []savedQueryEntry

	localDir, err := localQueriesDir()
	if err != nil {
		return nil, err
	}

	appendEntries := func(dir, source string) error {
		des, readErr := os.ReadDir(dir)
		if os.IsNotExist(readErr) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read queries dir %s: %w", dir, readErr)
		}
		for _, de := range des {
			if de.IsDir() {
				continue
			}
			n := de.Name()
			if !strings.HasSuffix(n, queryExt) {
				continue
			}
			entries = append(entries, savedQueryEntry{
				Name:   strings.TrimSuffix(n, queryExt),
				Source: source,
			})
		}
		return nil
	}

	if err := appendEntries(localDir, "local"); err != nil {
		return nil, err
	}
	if err := appendEntries(globalQueriesDir(), "global"); err != nil {
		return nil, err
	}

	return entries, nil
}

// deleteSavedQuery removes a query from the local or global store.
func deleteSavedQuery(name string, global bool) error {
	var dir string
	if global {
		dir = globalQueriesDir()
	} else {
		localDir, err := localQueriesDir()
		if err != nil {
			return err
		}
		dir = localDir
	}

	path := savedQueryPath(dir, name)
	if removeErr := os.Remove(path); removeErr != nil {
		if os.IsNotExist(removeErr) {
			scope := "local"
			if global {
				scope = "global"
			}
			return errs.NotFound("%s saved query %q not found", scope, name)
		}
		return fmt.Errorf("delete query %s: %w", path, removeErr)
	}
	return nil
}

// newQuerySaveCmd returns 'mcli query save <name>'.
func newQuerySaveCmd() *cobra.Command {
	var (
		queryFlag string
		fileFlag  string
		global    bool
	)

	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save a GraphQL query for later use",
		Long: `Save a GraphQL query under a name for use with 'mcli query run <name>'.

The query can be provided with --query, read from a file with -f, or piped
via stdin with -f -.

By default the query is saved locally (.mcli/queries/<name>.graphql in the
current directory). Use --global to save to ~/.config/mcli/queries/.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if queryFlag != "" && fileFlag != "" {
				return errs.Usage("cannot use both --query and -f")
			}
			queryStr, err := readQuerySource(queryFlag, fileFlag)
			if err != nil {
				return err
			}
			if queryStr == "" {
				return errs.Usage("query string must not be empty")
			}
			if err := saveSavedQuery(name, queryStr, global); err != nil {
				return err
			}
			scope := "local"
			if global {
				scope = "global"
			}
			_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "Saved %s query %q\n", scope, name)
			return writeErr
		},
	}

	cmd.Flags().StringVar(&queryFlag, "query", "", "inline GraphQL query string")
	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "read query from file ('-' for stdin)")
	cmd.Flags().BoolVar(&global, "global", false, "save to global store (~/.config/mcli/queries/)")

	return cmd
}

// newQueryListCmd returns 'mcli query list'.
func newQueryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved queries",
		Long:  "List all saved queries from the local and global stores.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := listSavedQueries()
			if err != nil {
				return err
			}

			mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
			if modeErr != nil {
				return modeErr
			}

			if mode == ModeJSON {
				if entries == nil {
					entries = []savedQueryEntry{}
				}
				type listOutput struct {
					Items []savedQueryEntry `json:"items"`
				}
				data, mErr := json.Marshal(listOutput{Items: entries})
				if mErr != nil {
					return fmt.Errorf("marshal output: %w", mErr)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return err
			}

			if len(entries) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "No saved queries found.")
				return err
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tSOURCE")
			for _, e := range entries {
				_, _ = fmt.Fprintf(w, "%s\t%s\n", e.Name, e.Source)
			}
			return w.Flush()
		},
	}
}

// newQueryDeleteCmd returns 'mcli query delete <name>'.
func newQueryDeleteCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved query",
		Long: `Delete a saved query by name.

By default deletes from the local store. Use --global to delete from the
global store (~/.config/mcli/queries/).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := deleteSavedQuery(name, global); err != nil {
				return err
			}
			scope := "local"
			if global {
				scope = "global"
			}
			_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s query %q\n", scope, name)
			return writeErr
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "delete from global store")

	return cmd
}
