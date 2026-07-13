package main

import (
	"github.com/spf13/cobra"

	"github.com/struktly/struktly/internal/mcp"
)

func newMCPCmd(repoRoot *string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve struktly over the Model Context Protocol (stdio)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return mcp.Serve(*repoRoot, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}
