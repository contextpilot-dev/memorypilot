package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "0.1.1"
	cfgFile string
)

var rootCmd = &cobra.Command{
	Use:   "memorypilot",
	Short: "One memory. Every AI. Zero repetition.",
	Long: `MemoryPilot is a passive, intelligent memory layer for AI-assisted development.

It automatically captures context from your work (git commits, file changes, 
terminal commands) and makes it available to any AI tool through MCP or REST API.

Your AI tools will finally remember you.`,
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.memorypilot/config.yaml)")
	
	// Add subcommands
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(recallCmd)
	rootCmd.AddCommand(rememberCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(mcpCmd)
}

// getConfigDir returns the MemoryPilot config directory
func getConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	return home + "/.memorypilot"
}

// getDataDir returns the MemoryPilot data directory
func getDataDir() string {
	return getConfigDir() + "/data"
}
