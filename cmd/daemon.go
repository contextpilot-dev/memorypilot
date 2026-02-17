package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/contextpilot-dev/memorypilot/internal/agent"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the MemoryPilot background daemon",
	Long:  `Start, stop, or check the status of the MemoryPilot background daemon.`,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the MemoryPilot daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🧠 Starting MemoryPilot daemon...")
		
		// Create and start the agent
		cfg := agent.DefaultConfig()
		cfg.DataDir = getDataDir()
		
		a, err := agent.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to create agent: %w", err)
		}
		
		// Start the agent
		if err := a.Start(); err != nil {
			return fmt.Errorf("failed to start agent: %w", err)
		}
		
		fmt.Println("✅ MemoryPilot daemon started")
		fmt.Println("   Watching for events...")
		fmt.Println("   Press Ctrl+C to stop")
		
		// Wait for shutdown signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		
		fmt.Println("\n🛑 Shutting down...")
		a.Stop()
		fmt.Println("✅ MemoryPilot daemon stopped")
		
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the MemoryPilot daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement daemon stop via PID file or IPC
		fmt.Println("Stopping MemoryPilot daemon...")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement status check
		fmt.Println("Checking MemoryPilot daemon status...")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
}
