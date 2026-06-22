package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/httpx"
	"github.com/mondaycom/mcli/internal/secrets"
)

var mondayAPIEndpoint = config.ResolveEndpoint()

// queryHTTPFactory is a test seam for the raw HTTP client used by query commands.
// Production code leaves this nil, causing newQueryHTTPClient to build a real client.
var queryHTTPFactory func() (*http.Client, string, error)

// newQueryHTTPClient returns an authenticated HTTP client and the API endpoint.
// If queryHTTPFactory is set (tests), it delegates there.
func newQueryHTTPClient() (*http.Client, string, error) {
	if queryHTTPFactory != nil {
		return queryHTTPFactory()
	}

	cfgPath := resolveConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, "", errs.Internal("load config: %v", err)
	}

	var store config.Store
	if cfg.SecretStore != "" {
		st, openErr := secrets.Open(cfg.SecretStore, resolveConfigDir())
		if openErr != nil {
			return nil, "", openErr
		}
		store = st
	}

	token, err := config.ResolveToken(cfg, globals.Token, store)
	if err != nil {
		return nil, "", err
	}

	client := httpx.NewClient(token, version)
	return client, mondayAPIEndpoint, nil
}

// jsonVarTypeRE matches variable declarations like "$cols: JSON!" or "$x: JSON"
// in a GraphQL operation signature. It captures the variable name and the base
// type (before any '!' or wrapping brackets).
var jsonVarTypeRE = regexp.MustCompile(`\$(\w+)\s*:\s*\[?\s*JSON\s*!?\s*\]?\s*!?`)

// coerceJSONVars inspects the query's variable declarations and, for any
// variable declared as a JSON scalar, re-encodes map/slice values as a JSON
// string. monday.com's JSON scalar expects a stringified JSON value on the wire.
func coerceJSONVars(queryStr string, vars map[string]any) {
	if len(vars) == 0 {
		return
	}
	matches := jsonVarTypeRE.FindAllStringSubmatch(queryStr, -1)
	for _, m := range matches {
		name := m[1]
		v, ok := vars[name]
		if !ok || v == nil {
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
			if b, err := json.Marshal(v); err == nil {
				vars[name] = string(b)
			}
		}
	}
}

// parseVarValue auto-types a value string per the spec:
// int64 → float64 → bool → null → JSON object/array → string.
func parseVarValue(s string) any {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	switch s {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err == nil {
			return v
		}
	}
	return s
}

// parseVarFlags parses repeated --var key=value flags into a map.
// Later values win on duplicate keys. Returns errs.Usage on bad syntax.
func parseVarFlags(varFlags []string) (map[string]any, error) {
	out := make(map[string]any, len(varFlags))
	for _, flag := range varFlags {
		k, v, ok := strings.Cut(flag, "=")
		if !ok {
			return nil, errs.Usage("--var %q: must be in key=value form", flag)
		}
		if k == "" {
			return nil, errs.Usage("--var flag has empty key in %q", flag)
		}
		out[k] = parseVarValue(v)
	}
	return out, nil
}

// mergeVarsFile reads a JSON object from path and merges it into dst.
// Keys from dst (--var flags) take priority over the file.
func mergeVarsFile(dst map[string]any, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return errs.Usage("read vars file %q: %v", path, err)
	}
	var fileVars map[string]any
	if err := json.Unmarshal(data, &fileVars); err != nil {
		return errs.Usage("--vars-file %q: not a valid JSON object: %v", path, err)
	}
	for k, v := range fileVars {
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
	return nil
}

// readQuerySource reads a GraphQL query string from either a positional arg,
// a -f file flag, or stdin when -f is "-".
// Exactly one of queryArg or fileFlag should be non-empty.
func readQuerySource(queryArg, fileFlag string) (string, error) {
	if queryArg != "" && fileFlag != "" {
		return "", errs.Usage("cannot use both an inline query argument and -f")
	}
	if queryArg != "" {
		return queryArg, nil
	}
	if fileFlag == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read query from stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	if fileFlag != "" {
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			return "", fmt.Errorf("read query file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return "", errs.Usage("provide a query as an argument or use -f <file>")
}

// executeRawQuery sends a raw GraphQL query to the monday.com API and writes
// the response JSON to cmd's stdout. It exits with code 2 when the response
// contains a top-level "errors" key.
func executeRawQuery(cmd *cobra.Command, queryStr string, vars map[string]any) error {
	if queryStr == "" {
		return errs.Usage("query string must not be empty")
	}

	coerceJSONVars(queryStr, vars)

	httpClient, endpoint, err := newQueryHTTPClient()
	if err != nil {
		return err
	}

	payload := map[string]any{"query": queryStr}
	if len(vars) > 0 {
		payload["variables"] = vars
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return errs.Internal("marshal query payload: %v", err)
	}

	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errs.Internal("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return errs.API("execute query: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return errs.API("read response: %v", err)
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
	if err != nil {
		return errs.Internal("write output: %v", err)
	}

	// Check for GraphQL-level errors to set exit code 2.
	var parsed struct {
		Errors json.RawMessage `json:"errors"`
	}
	if jsonErr := json.Unmarshal(respBody, &parsed); jsonErr == nil {
		if len(parsed.Errors) > 0 && string(parsed.Errors) != "null" {
			return errs.API("query returned errors")
		}
	}

	return nil
}

// newQueryCmd returns the 'mcli query' parent command.
func newQueryCmd() *cobra.Command {
	var (
		fileFlag string
		varFlags []string
		varsFile string
	)

	cmd := &cobra.Command{
		Use:   "query [<graphql>]",
		Short: "Execute raw GraphQL queries against the monday.com API",
		Long: `Execute a raw GraphQL query against the monday.com API.

The query string can be provided as a positional argument, read from a file
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

	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "read query from file ('-' for stdin)")
	cmd.Flags().StringArrayVar(&varFlags, "var", nil, "variable: key=value (repeatable, auto-typed)")
	cmd.Flags().StringVar(&varsFile, "vars-file", "", "JSON file with variables object")

	cmd.AddCommand(newQueryRunCmd())
	cmd.AddCommand(newQuerySaveCmd())
	cmd.AddCommand(newQueryListCmd())
	cmd.AddCommand(newQueryDeleteCmd())

	return cmd
}

// newQueryRunCmd returns 'mcli query run <name>'.
func newQueryRunCmd() *cobra.Command {
	var (
		varFlags []string
		varsFile string
	)

	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a saved query by name",
		Long: `Run a previously saved query by name.

Looks up local (.mcli/queries/<name>.graphql) before global
(~/.config/mcli/queries/<name>.graphql).

Variables are passed with --var key=value and/or --vars-file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			queryStr, err := loadSavedQuery(name)
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
