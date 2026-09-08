package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/config"
	"github.com/mondaycom/mcli/internal/errs"
	"github.com/mondaycom/mcli/internal/secrets"
	rawschema "github.com/mondaycom/mcli/schema"
)

// staleAfterDays is how old the live schema may get before mcli warns. monday
// ships API versions quarterly and adds fields within a version continuously, so
// a month is a reasonable "you are probably missing operations" threshold.
const staleAfterDays = 30

// maxDeltaNames caps how many added/removed type names a refresh prints before
// collapsing to a count, so the output stays readable when a version bump lands.
const maxDeltaNames = 10

func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Inspect and refresh the local monday.com GraphQL schema",
		Long: `Inspect and refresh the monday.com GraphQL schema that drives 'mcli api'.

mcli ships with an embedded schema. Refreshing fetches the live schema with your
own API token and caches it locally, so you pick up new monday operations without
waiting for an mcli release.`,
	}
	cmd.AddCommand(newSchemaRefreshCmd())
	cmd.AddCommand(newSchemaStatusCmd())
	return cmd
}

func newSchemaRefreshCmd() *cobra.Command {
	var apiVersion string

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Fetch the live schema and cache it locally",
		Long: `Fetch the monday.com GraphQL schema by introspection and cache it locally.

Uses your configured API token and API version. The cached schema takes precedence
over the embedded one for all 'mcli api' operations.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSchemaRefresh(cmd, apiVersion)
		},
	}

	cmd.Flags().StringVar(&apiVersion, "api-version", "",
		"fetch this API version instead of the configured one (not persisted)")

	return cmd
}

func newSchemaStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report which schema is in use and how old it is",
		Args:  cobra.NoArgs,
		RunE:  runSchemaStatus,
	}
}

// schemaStatus is the JSON shape for 'mcli schema status'.
type schemaStatus struct {
	// Source is "cached" when a locally refreshed schema is in use, "embedded"
	// when the copy compiled into the binary is.
	Source     string `json:"source"`
	Path       string `json:"path,omitempty"`
	APIVersion string `json:"api_version"`
	// FetchedAt is YYYY-MM-DD, or empty when the date cannot be determined.
	FetchedAt string `json:"fetched_at,omitempty"`
	// AgeDays is -1 when FetchedAt is unknown.
	AgeDays int  `json:"age_days"`
	Stale   bool `json:"stale"`
	Types   int  `json:"types"`
}

// liveSchemaInfo reports which schema apischema.Load would use and how old it is.
// A cached schema's age comes from its file mtime; the embedded schema's from the
// date recorded at `make schema` time.
func liveSchemaInfo(cfg config.Config) schemaStatus {
	st := schemaStatus{
		Source:     apischema.SourceEmbedded,
		APIVersion: config.ResolveAPIVersion(cfg),
		AgeDays:    -1,
	}

	if s, err := apischema.Load(); err == nil && s != nil {
		st.Types = len(s.Types)
	}
	// Ask apischema which SDL it parsed rather than inferring it from the file's
	// existence: an unparseable cache falls back to the embedded copy.
	if src, err := apischema.LoadedSource(); err == nil {
		st.Source = src
	}

	var fetched time.Time
	if st.Source == apischema.SourceCached {
		st.Path = apischema.CachedSchemaPath(resolveConfigDir())
		if fi, err := os.Stat(st.Path); err == nil {
			fetched = fi.ModTime()
		}
	} else {
		fetched = rawschema.FetchedAt()
	}

	if !fetched.IsZero() {
		st.FetchedAt = fetched.Format("2006-01-02")
		st.AgeDays = int(time.Since(fetched).Hours() / 24)
		st.Stale = st.AgeDays > staleAfterDays
	}

	return st
}

// warnIfSchemaStale emits a one-line staleness warning to stderr.
//
// It deliberately never fetches: mcli is driven by LLM agents in steady state, and
// an implicit network call on a read path can hang, fail, or need a token that is
// not there. It also never writes to stdout, which carries the JSON contract.
func warnIfSchemaStale(cmd *cobra.Command) {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		return
	}
	st := liveSchemaInfo(cfg)
	if !st.Stale {
		return
	}
	// A failed warning is not worth reporting: it must never affect the command's
	// own exit status.
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"warning: %s monday schema is %d days old; run 'mcli schema refresh' to update\n",
		st.Source, st.AgeDays)
}

// resolveAPIToken resolves the API token with the standard precedence:
// --token flag, then MONDAY_API_TOKEN, then the configured secret store.
func resolveAPIToken(cfg config.Config) (config.APIToken, error) {
	var store config.Store
	if cfg.SecretStore != "" {
		st, openErr := secrets.Open(cfg.SecretStore, resolveConfigDir())
		if openErr != nil {
			return "", openErr
		}
		store = st
	}
	return config.ResolveToken(cfg, globals.Token, store)
}

// fetchAndCacheSchema introspects the live schema for apiVersion, writes it to the
// local cache and busts the in-memory cache so the new SDL takes effect
// immediately. It is the single implementation shared by 'mcli schema refresh' and
// 'mcli config set api-version'.
func fetchAndCacheSchema(ctx context.Context, cfg config.Config, apiVersion string, token config.APIToken) (string, error) {
	sdl, err := apischema.FetchSchema(ctx, token, config.ResolveEndpoint(cfg), apiVersion)
	if err != nil {
		return "", err
	}

	path := apischema.CachedSchemaPath(resolveConfigDir())
	if err := os.WriteFile(path, []byte(sdl), 0o600); err != nil {
		return "", errs.Internal("write schema cache: %v", err)
	}

	apischema.SetConfigDir(resolveConfigDir())
	return path, nil
}

// schemaTypeNames returns the set of type names in the currently loaded schema.
// It returns nil if the schema cannot be loaded, so a refresh still succeeds when
// only the delta summary is unavailable.
func schemaTypeNames() map[string]bool {
	s, err := apischema.Load()
	if err != nil || s == nil {
		return nil
	}
	names := make(map[string]bool, len(s.Types))
	for name := range s.Types {
		names[name] = true
	}
	return names
}

// diffNames returns names present in b but not a, sorted.
func diffNames(a, b map[string]bool) []string {
	var out []string
	for name := range b {
		if !a[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// schemaRefreshOutput is the JSON shape for 'mcli schema refresh'.
type schemaRefreshOutput struct {
	Path       string   `json:"path"`
	APIVersion string   `json:"api_version"`
	Types      int      `json:"types"`
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
}

func runSchemaRefresh(cmd *cobra.Command, apiVersionOverride string) error {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		return errs.Internal("load config: %v", err)
	}

	apiVersion := config.ResolveAPIVersion(cfg)
	if apiVersionOverride != "" {
		if !apiVersionRE.MatchString(apiVersionOverride) {
			return errs.Usage("invalid api-version %q: must be YYYY-MM (e.g. 2026-07) or a named version (e.g. dev)", apiVersionOverride)
		}
		apiVersion = apiVersionOverride
	}

	token, err := resolveAPIToken(cfg)
	if err != nil {
		return err
	}

	before := schemaTypeNames()

	path, err := fetchAndCacheSchema(cmd.Context(), cfg, apiVersion, token)
	if err != nil {
		return err
	}

	after := schemaTypeNames()

	out := schemaRefreshOutput{
		Path:       path,
		APIVersion: apiVersion,
		Types:      len(after),
		Added:      diffNames(before, after),
		Removed:    diffNames(after, before),
	}

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	if mode == ModeJSON {
		data, mErr := json.Marshal(out)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	}

	o := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(o, "fetched schema for %s → %s\n", apiVersion, path)
	_, _ = fmt.Fprintf(o, "types: %d\n", out.Types)
	printDelta(o, "added", out.Added)
	printDelta(o, "removed", out.Removed)
	if len(out.Added) == 0 && len(out.Removed) == 0 && before != nil {
		_, _ = fmt.Fprintln(o, "no type changes")
	}
	return nil
}

// printDelta writes a "label: a, b, c" line, collapsing to a count past
// maxDeltaNames. It prints nothing for an empty delta.
func printDelta(w interface{ Write([]byte) (int, error) }, label string, names []string) {
	if len(names) == 0 {
		return
	}
	if len(names) > maxDeltaNames {
		_, _ = fmt.Fprintf(w, "%s: %d types\n", label, len(names))
		return
	}
	for _, n := range names {
		_, _ = fmt.Fprintf(w, "%s: %s\n", label, n)
	}
}

func runSchemaStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		return errs.Internal("load config: %v", err)
	}

	st := liveSchemaInfo(cfg)

	mode, modeErr := resolveOutputMode(os.Stdout, globals, configOutputMode())
	if modeErr != nil {
		return modeErr
	}

	switch mode {
	case ModeJSON:
		data, mErr := json.Marshal(st)
		if mErr != nil {
			return errs.Internal("marshal output: %v", mErr)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err

	case ModeTerse:
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s age=%dd types=%d stale=%t\n",
			st.Source, st.APIVersion, st.AgeDays, st.Types, st.Stale)
		return err

	default:
		o := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(o, "source:      %s\n", st.Source)
		if st.Path != "" {
			_, _ = fmt.Fprintf(o, "path:        %s\n", st.Path)
		}
		_, _ = fmt.Fprintf(o, "api-version: %s\n", st.APIVersion)
		if st.FetchedAt != "" {
			_, _ = fmt.Fprintf(o, "fetched:     %s (%d days ago)\n", st.FetchedAt, st.AgeDays)
		} else {
			_, _ = fmt.Fprintf(o, "fetched:     unknown\n")
		}
		_, _ = fmt.Fprintf(o, "types:       %d\n", st.Types)
		if st.Stale {
			_, _ = fmt.Fprintf(o, "\nschema is stale; run 'mcli schema refresh' to update\n")
		}
		return nil
	}
}
