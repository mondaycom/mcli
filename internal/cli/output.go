package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mondaycom/mcli/internal/errs"
)

// OutputMode determines how a command's output and errors are rendered.
type OutputMode int

const (
	// ModeJSON emits a single JSON object per invocation to stdout.
	ModeJSON OutputMode = iota
	// ModePretty emits human-readable text to stdout for results, stderr for errors.
	ModePretty
)

// resolveOutputMode picks the effective output mode using globals and a TTY probe.
// Precedence:
//  1. If both --json and --pretty are set → USAGE error.
//  2. If --json is set → ModeJSON.
//  3. If --pretty is set → ModePretty.
//  4. Otherwise: ModePretty when stdout is a TTY, ModeJSON when not.
func resolveOutputMode(stdout *os.File, g GlobalFlags) (OutputMode, error) {
	if g.JSON && g.Pretty {
		return 0, errs.Usage("--json and --pretty are mutually exclusive")
	}
	if g.JSON {
		return ModeJSON, nil
	}
	if g.Pretty {
		return ModePretty, nil
	}
	if isCharDevice(stdout) {
		return ModePretty, nil
	}
	return ModeJSON, nil
}

// isCharDevice reports whether f is attached to a character device (i.e. a TTY).
func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// PrintError renders err per ADR-002:
//   - In ModeJSON the structured JSON object is the last line of stdout.
//   - In ModePretty a human-readable message is written to stderr.
//
// This is intended for use from main after Execute() returns a non-nil error.
func PrintError(err error) {
	if err == nil {
		return
	}
	mode, modeErr := resolveOutputMode(os.Stdout, globals)
	if modeErr != nil {
		// If mode resolution itself failed (conflicting flags), prefer JSON to
		// stderr so the error is still machine-parseable.
		writeJSONError(os.Stderr, modeErr)
		return
	}
	switch mode {
	case ModeJSON:
		writeJSONError(os.Stdout, err)
	case ModePretty:
		writePrettyError(os.Stderr, err)
	}
}

func writeJSONError(w io.Writer, err error) {
	payload := map[string]any{"error": errorPayload(err)}
	data, mErr := json.Marshal(payload)
	if mErr != nil {
		// Fallback: avoid swallowing the original error if JSON marshalling
		// fails (it shouldn't, but stay safe).
		_, _ = fmt.Fprintf(w, "{\"error\":{\"code\":\"INTERNAL\",\"message\":%q}}\n", err.Error())
		return
	}
	_, _ = fmt.Fprintln(w, string(data))
}

func writePrettyError(w io.Writer, err error) {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		_, _ = fmt.Fprintf(w, "Error [%s]: %s\n", e.Code, e.Message)
		return
	}
	_, _ = fmt.Fprintf(w, "Error: %v\n", err)
}

func errorPayload(err error) map[string]any {
	if e, ok := errors.AsType[*errs.Error](err); ok {
		return map[string]any{
			"code":    string(e.Code),
			"message": e.Message,
		}
	}
	return map[string]any{
		"code":    string(errs.CodeInternal),
		"message": err.Error(),
	}
}
