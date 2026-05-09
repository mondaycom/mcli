// Command mcli is the monday.com CLI.
package main

import (
	"os"

	"github.com/mondaycom/mcli/internal/cli"
	"github.com/mondaycom/mcli/internal/errs"
)

func main() {
	if err := cli.Execute(); err != nil {
		cli.PrintError(err)
		os.Exit(errs.ToExitCode(err))
	}
}
