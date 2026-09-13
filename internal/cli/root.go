// Package cli adapts command-line input to Archiver application behavior.
package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// BuildInfo identifies the CLI build displayed by the version command.
type BuildInfo struct {
	Version  string
	Revision string
}

// Execute runs the Archiver command tree with the supplied arguments and streams.
func Execute(args []string, stdout, stderr io.Writer, buildInfo BuildInfo) int {
	root := NewRootCommand(buildInfo)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	return 0
}

// NewRootCommand creates the Archiver Cobra command tree.
func NewRootCommand(buildInfo BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "archiver",
		Short:         "Create encrypted, multi-disc archive images",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newVersionCommand(buildInfo))
	return root
}

func newVersionCommand(buildInfo BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the CLI version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), formatVersion(buildInfo))
			return err
		},
	}
}

func formatVersion(buildInfo BuildInfo) string {
	if buildInfo.Revision == "" {
		return fmt.Sprintf("archiver %s", buildInfo.Version)
	}

	return fmt.Sprintf("archiver %s (%s)", buildInfo.Version, buildInfo.Revision)
}
