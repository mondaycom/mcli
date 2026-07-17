// Command mcli is the monday.com CLI.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mondaycom/mcli/internal/cli"
	"github.com/mondaycom/mcli/internal/errs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cli.Execute(ctx); err != nil {
		cli.PrintError(err)
		os.Exit(errs.ToExitCode(err))
	}
}
