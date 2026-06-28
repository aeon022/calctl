package cmd

import (
	"github.com/aeon022/calctl/internal/mcpserver"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the calctl MCP server (stdio)",
	Long: `Start a Model Context Protocol server on stdio.

Add to your Claude Code MCP config (~/.claude.json):

  {
    "mcpServers": {
      "calctl": {
        "command": "/usr/local/bin/calctl",
        "args": ["mcp"]
      }
    }
  }

Claude can then call: today, this_week, list_events, sync, find_free_slots, create_event.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return mcpserver.Serve()
	},
	// don't show usage on error — MCP servers write to stdout, usage would corrupt the stream
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
