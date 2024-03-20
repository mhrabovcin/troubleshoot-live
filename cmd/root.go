// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/mesosphere/dkp-cli-runtime/core/cmd/root"
	"github.com/mesosphere/dkp-cli-runtime/core/output"
)

// NewCommand creates root command.
func NewCommand(in io.Reader, out, errOut io.Writer) (*cobra.Command, output.Output) {
	rootCmd, rootOpts := root.NewCommand(out, errOut)

	// Enable structured logging
	slog.SetDefault(slog.New(slog.NewTextHandler(rootOpts.Output.V(1).InfoWriter(), &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	rootCmd.AddCommand(NewServeCommand(rootOpts.Output))

	return rootCmd, rootOpts.Output
}

// Execute runs the default CLI configuration.
func Execute() {
	rootCmd, out := NewCommand(os.Stdin, os.Stdout, os.Stderr)
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		out.Error(err, "")
		os.Exit(1)
	}
}
