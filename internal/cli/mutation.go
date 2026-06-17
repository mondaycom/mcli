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

const mutationsSubDir = "mutations"

func localMutationsDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Join(wd, ".mcli", mutationsSubDir), nil
}

func globalMutationsDir() string {
	return filepath.Join(resolveConfigDir(), mutationsSubDir)
}

func savedMutationPath(dir, name string) string {
	return filepath.Join(dir, name+queryExt)
}

func saveMutation(name, mutationStr string, global bool) error {
	var dir string
	if global {
		dir = globalMutationsDir()
	} else {
		localDir, err := localMutationsDir()
		if err != nil {
			return err
		}
		dir = localDir
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create mutations dir %s: %w", dir, err)
	}

	path := savedMutationPath(dir, name)
	if err := os.WriteFile(path, []byte(mutationStr), 0o600); err != nil {
		return fmt.Errorf("write mutation %s: %w", path, err)
	}
	return nil
}

func loadSavedMutation(name string) (string, error) {
	localDir, err := localMutationsDir()
	if err != nil {
		return "", err
	}

	localPath := savedMutationPath(localDir, name)
	if data, err := os.ReadFile(localPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	globalPath := savedMutationPath(globalMutationsDir(), name)
	if data, err := os.ReadFile(globalPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	return "", errs.NotFound("saved mutation %q not found (checked local and global)", name)
}

type savedMutationEntry struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

func listSavedMutations() ([]savedMutationEntry, error) {
	var entries []savedMutationEntry

	localDir, err := localMutationsDir()
	if err != nil {
		return nil, err
	}

	appendEntries := func(dir, source string) error {
		des, readErr := os.ReadDir(dir)
		if os.IsNotExist(readErr) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read mutations dir %s: %w", dir, readErr)
		}
		for _, de := range des {
			if de.IsDir() {
				continue
			}
			n := de.Name()
			if !strings.HasSuffix(n, queryExt) {
				continue
			}
			entries = append(entries, savedMutationEntry{
				Name:   strings.TrimSuffix(n, queryExt),
				Source: source,
			})
		}
		return nil
	}

	if err := appendEntries(localDir, "local"); err != nil {
		return nil, err
	}
	if err := appendEntries(globalMutationsDir(), "global"); err != nil {
		return nil, err
	}

	return entries, nil
}

func deleteSavedMutation(name string, global bool) error {
	var dir string
	if global {
		dir = globalMutationsDir()
	} else {
		localDir, err := localMutationsDir()
		if err != nil {
			return err
		}
		dir = localDir
	}

	path := savedMutationPath(dir, name)
	if removeErr := os.Remove(path); removeErr != nil {
		if os.IsNotExist(removeErr) {
			scope := "local"
			if global {
				scope = "global"
			}
			return errs.NotFound("%s saved mutation %q not found", scope, name)
		}
		return fmt.Errorf("delete mutation %s: %w", path, removeErr)
	}
	return nil
}

// newMutationCmd returns the 'mcli mutation' parent command.
func newMutationCmd() *cobra.Command {
	var (
		fileFlag string
		varFlags []string
		varsFile string
	)

	cmd := &cobra.Command{
		Use:   "mutation [<graphql>]",
		Short: "Execute raw GraphQL mutations against the monday.com API",
		Long: `Execute a raw GraphQL mutation against the monday.com API.

The mutation string can be provided as a positional argument, read from a file
with -f, or piped via stdin with -f -.

Variables are passed with --var key=value (repeatable) and/or --vars-file
pointing to a JSON object. When both are used, --var values win on conflicts.

Variable values are auto-typed: integers, floats, booleans, null, JSON objects
and arrays are parsed accordingly; everything else becomes a string.

Exit code 0 means the response has no errors; exit code 2 means errors were
returned (the response body is still printed).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var queryArg string
			if len(args) == 1 {
				queryArg = args[0]
			}
			queryStr, err := readQuerySource(queryArg, fileFlag)
			if err != nil {
				return err
			}
			vars, err := parseVarFlags(varFlags)
			if err != nil {
				return err
			}
			if varsFile != "" {
				if err := mergeVarsFile(vars, varsFile); err != nil {
					return err
				}
			}
			return executeRawQuery(cmd, queryStr, vars)
		},
	}

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "read mutation from file ('-' for stdin)")
	cmd.Flags().StringArrayVar(&varFlags, "var", nil, "variable: key=value (repeatable, auto-typed)")
	cmd.Flags().StringVar(&varsFile, "vars-file", "", "JSON file with variables object")

	cmd.AddCommand(newMutationRunCmd())
	cmd.AddCommand(newMutationSaveCmd())
	cmd.AddCommand(newMutationListCmd())
	cmd.AddCommand(newMutationDeleteCmd())

	return cmd
}

func newMutationRunCmd() *cobra.Command {
	var (
		varFlags []string
		varsFile string
	)

	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a saved mutation by name",
		Long: `Run a previously saved mutation by name.

Looks up local (.mcli/mutations/) first, then global (~/.config/mcli/mutations/).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			queryStr, err := loadSavedMutation(name)
			if err != nil {
				return err
			}
			vars, err := parseVarFlags(varFlags)
			if err != nil {
				return err
			}
			if varsFile != "" {
				if err := mergeVarsFile(vars, varsFile); err != nil {
					return err
				}
			}
			return executeRawQuery(cmd, queryStr, vars)
		},
	}

	cmd.Flags().StringArrayVar(&varFlags, "var", nil, "variable: key=value (repeatable, auto-typed)")
	cmd.Flags().StringVar(&varsFile, "vars-file", "", "JSON file with variables object")

	return cmd
}

func newMutationSaveCmd() *cobra.Command {
	var (
		queryFlag string
		fileFlag  string
		global    bool
	)

	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save a GraphQL mutation for later use",
		Long: `Save a GraphQL mutation under a name for use with 'mcli mutation run <name>'.

The mutation can be provided with --query, read from a file with -f, or piped
via stdin with -f -.

By default the mutation is saved locally (.mcli/mutations/<name>.graphql in the
current directory). Use --global to save to ~/.config/mcli/mutations/.`,
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
				return errs.Usage("mutation string must not be empty")
			}
			if err := saveMutation(name, queryStr, global); err != nil {
				return err
			}
			scope := "local"
			if global {
				scope = "global"
			}
			_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "Saved %s mutation %q\n", scope, name)
			return writeErr
		},
	}

	cmd.Flags().StringVar(&queryFlag, "query", "", "inline GraphQL mutation string")
	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "read mutation from file ('-' for stdin)")
	cmd.Flags().BoolVar(&global, "global", false, "save to global store (~/.config/mcli/mutations/)")

	return cmd
}

func newMutationListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved mutations",
		Long:  "List all saved mutations from the local and global stores.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			entries, err := listSavedMutations()
			if err != nil {
				return err
			}

			mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
			if modeErr != nil {
				return modeErr
			}

			if mode == ModeJSON {
				if entries == nil {
					entries = []savedMutationEntry{}
				}
				type listOutput struct {
					Items []savedMutationEntry `json:"items"`
				}
				data, mErr := json.Marshal(listOutput{Items: entries})
				if mErr != nil {
					return fmt.Errorf("marshal output: %w", mErr)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return err
			}

			if len(entries) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "No saved mutations found.")
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

func newMutationDeleteCmd() *cobra.Command {
	var global bool

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved mutation",
		Long: `Delete a saved mutation by name.

By default deletes from the local store. Use --global to delete from the
global store (~/.config/mcli/mutations/).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := deleteSavedMutation(name, global); err != nil {
				return err
			}
			scope := "local"
			if global {
				scope = "global"
			}
			_, writeErr := fmt.Fprintf(cmd.OutOrStdout(), "Deleted %s mutation %q\n", scope, name)
			return writeErr
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "delete from global store")

	return cmd
}
