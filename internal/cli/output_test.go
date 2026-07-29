package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/errs"
)

func TestRenderError_JSONToStdout(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	renderError(ModeJSON, &out, &errBuf, errs.Usage("bad id %q", "abc"))

	if errBuf.Len() != 0 {
		t.Errorf("JSON mode must not write to stderr, got %q", errBuf.String())
	}
	got := out.String()
	if !strings.Contains(got, `"code":"USAGE"`) || !strings.Contains(got, `bad id \"abc\"`) {
		t.Errorf("expected structured JSON error on stdout, got %q", got)
	}
}

// TestRenderError_NonJSONModesSurfaceError guards the A5 regression: terse and
// csv modes previously fell through PrintError's switch and dropped the error
// entirely. Every non-JSON mode must render a human-readable message to stderr.
func TestRenderError_NonJSONModesSurfaceError(t *testing.T) {
	t.Parallel()

	for _, mode := range []OutputMode{ModePretty, ModeTerse, ModeCSV} {
		mode := mode
		t.Run(modeName(mode), func(t *testing.T) {
			t.Parallel()
			var out, errBuf bytes.Buffer
			renderError(mode, &out, &errBuf, errs.Usage("nope"))

			if out.Len() != 0 {
				t.Errorf("non-JSON mode must not write to stdout, got %q", out.String())
			}
			if got := errBuf.String(); !strings.Contains(got, "Error [USAGE]: nope") {
				t.Errorf("expected human-readable error on stderr, got %q", got)
			}
		})
	}
}

func TestRenderError_NilIsNoop(t *testing.T) {
	t.Parallel()

	var out, errBuf bytes.Buffer
	renderError(ModeTerse, &out, &errBuf, nil)
	if out.Len() != 0 || errBuf.Len() != 0 {
		t.Errorf("nil error must produce no output, got stdout=%q stderr=%q", out.String(), errBuf.String())
	}
}

func modeName(m OutputMode) string {
	switch m {
	case ModeJSON:
		return "json"
	case ModePretty:
		return "pretty"
	case ModeTerse:
		return "terse"
	case ModeCSV:
		return "csv"
	default:
		return "unknown"
	}
}

func TestResolveOutputMode_FlagConflict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		g    GlobalFlags
	}{
		{"json+pretty", GlobalFlags{JSON: true, Pretty: true}},
		{"json+terse", GlobalFlags{JSON: true, Terse: true}},
		{"json+csv", GlobalFlags{JSON: true, CSV: true}},
		{"pretty+terse", GlobalFlags{Pretty: true, Terse: true}},
		{"pretty+csv", GlobalFlags{Pretty: true, CSV: true}},
		{"terse+csv", GlobalFlags{Terse: true, CSV: true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := resolveOutputMode(os.Stdout, tc.g, ""); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestResolveOutputMode_JSONFlag(t *testing.T) {
	t.Parallel()

	mode, err := resolveOutputMode(os.Stdout, GlobalFlags{JSON: true}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Errorf("expected ModeJSON, got %v", mode)
	}
}

func TestResolveOutputMode_PrettyFlag(t *testing.T) {
	t.Parallel()

	mode, err := resolveOutputMode(os.Stdout, GlobalFlags{Pretty: true}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModePretty {
		t.Errorf("expected ModePretty, got %v", mode)
	}
}

func TestResolveOutputMode_TerseFlag(t *testing.T) {
	t.Parallel()

	mode, err := resolveOutputMode(os.Stdout, GlobalFlags{Terse: true}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeTerse {
		t.Errorf("expected ModeTerse, got %v", mode)
	}
}

func TestResolveOutputMode_CSVFlag(t *testing.T) {
	t.Parallel()

	mode, err := resolveOutputMode(os.Stdout, GlobalFlags{CSV: true}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeCSV {
		t.Errorf("expected ModeCSV, got %v", mode)
	}
}

func TestResolveOutputMode_FlagBeatsConfig(t *testing.T) {
	t.Parallel()

	mode, err := resolveOutputMode(os.Stdout, GlobalFlags{JSON: true}, "pretty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Errorf("flag should beat config: expected ModeJSON, got %v", mode)
	}
}

func TestResolveOutputMode_ConfigDefault(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	cases := []struct {
		configMode string
		want       OutputMode
	}{
		{"json", ModeJSON},
		{"pretty", ModePretty},
		{"terse", ModeTerse},
		{"csv", ModeCSV},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.configMode, func(t *testing.T) {
			mode, err := resolveOutputMode(w, GlobalFlags{}, tc.configMode)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tc.want {
				t.Errorf("config=%q: expected %v, got %v", tc.configMode, tc.want, mode)
			}
		})
	}
}

func TestResolveOutputMode_NonTTYDefaultsToJSON(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	mode, err := resolveOutputMode(w, GlobalFlags{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Errorf("non-TTY default should be ModeJSON, got %v", mode)
	}
}

func TestResolveOutputMode_UnknownConfigIgnored(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	mode, err := resolveOutputMode(w, GlobalFlags{}, "bogus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Errorf("unknown config value should fall through to TTY-detect (JSON for pipe), got %v", mode)
	}
}
