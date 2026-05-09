package cli

import (
	"os"
	"testing"
)

func TestResolveOutputMode_FlagConflict(t *testing.T) {
	t.Parallel()

	g := GlobalFlags{JSON: true, Pretty: true}
	if _, err := resolveOutputMode(os.Stdout, g); err == nil {
		t.Fatal("expected error when --json and --pretty are both set")
	}
}

func TestResolveOutputMode_JSONFlag(t *testing.T) {
	t.Parallel()

	g := GlobalFlags{JSON: true}
	mode, err := resolveOutputMode(os.Stdout, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Errorf("expected ModeJSON, got %v", mode)
	}
}

func TestResolveOutputMode_PrettyFlag(t *testing.T) {
	t.Parallel()

	g := GlobalFlags{Pretty: true}
	mode, err := resolveOutputMode(os.Stdout, g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModePretty {
		t.Errorf("expected ModePretty, got %v", mode)
	}
}

func TestResolveOutputMode_NonTTYDefaultsToJSON(t *testing.T) {
	t.Parallel()

	// A pipe is not a character device; t.TempDir backed file is also not.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	mode, err := resolveOutputMode(w, GlobalFlags{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != ModeJSON {
		t.Errorf("non-TTY default should be ModeJSON, got %v", mode)
	}
}
