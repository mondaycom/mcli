// Command mcli is the monday.com CLI.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/mondaycom/mcli/internal/cli"
	"github.com/mondaycom/mcli/internal/errs"
)

func main() {
	if err := cli.Execute(); err != nil {
		printError(err)
		os.Exit(errs.ToExitCode(err))
	}
}

// printError writes a structured error to stderr.
// When stdout is not a TTY (or --json is active) callers should also emit the
// JSON payload to stdout, but at the main entry point we don't have the output
// mode resolved yet — we always write to stderr here to stay safe.
func printError(err error) {
	var e *errs.Error
	if errors.As(err, &e) {
		payload := map[string]any{
			"error": map[string]any{
				"code":    string(e.Code),
				"message": e.Message,
			},
		}
		if data, jsonErr := json.Marshal(payload); jsonErr == nil {
			fmt.Fprintln(os.Stderr, string(data))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
}
