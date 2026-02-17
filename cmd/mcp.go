package cmd

import (
	"fmt"

	"github.com/contextpilot-dev/memorypilot/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server",
	Long: `Start the Model Context Protocol server for AI tool integration.

This is typically spawned by AI tools like Claude Code or OpenClaw.
The server communicates over stdio using the MCP protocol.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir := getDataDir()
		dbPath := dataDir + "/memories.db"
		
		server, err := mcp.NewServer(dbPath)
		if err != nil {
			return fmt.Errorf("failed to create MCP server: %w", err)
		}
		
		// Run the server (blocks until stdin closes)
		return server.Run()
	},
}
