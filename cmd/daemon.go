package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/contextpilot-dev/memorypilot/internal/agent"
	"github.com/spf13/cobra"
)

func getPidFilePath() string {
	return filepath.Join(getConfigDir(), "memorypilot.pid")
}

func writePidFile(pid int) error {
	return os.WriteFile(getPidFilePath(), []byte(strconv.Itoa(pid)), 0644)
}

func readPidFile() (int, error) {
	data, err := os.ReadFile(getPidFilePath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func removePidFile() {
	os.Remove(getPidFilePath())
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check if process exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the MemoryPilot background daemon",
	Long:  `Start, stop, or check the status of the MemoryPilot background daemon.`,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the MemoryPilot daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		background, _ := cmd.Flags().GetBool("background")
		
		// Check if already running
		if pid, err := readPidFile(); err == nil {
			if isProcessRunning(pid) {
				fmt.Printf("❌ MemoryPilot daemon already running (PID %d)\n", pid)
				return nil
			}
			// Stale PID file, remove it
			removePidFile()
		}
		
		if background {
			// Start as background process
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to get executable path: %w", err)
			}
			
			bgCmd := exec.Command(exe, "daemon", "start")
			bgCmd.Stdout = nil
			bgCmd.Stderr = nil
			bgCmd.Stdin = nil
			bgCmd.SysProcAttr = &syscall.SysProcAttr{
				Setsid: true, // Create new session (detach from terminal)
			}
			
			if err := bgCmd.Start(); err != nil {
				return fmt.Errorf("failed to start background process: %w", err)
			}
			
			fmt.Printf("✅ MemoryPilot daemon started (PID %d)\n", bgCmd.Process.Pid)
			fmt.Println("   Use 'memorypilot daemon status' to check")
			fmt.Println("   Use 'memorypilot daemon stop' to stop")
			return nil
		}
		
		fmt.Println("🧠 Starting MemoryPilot daemon...")
		
		// Write PID file
		if err := writePidFile(os.Getpid()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to write PID file: %v\n", err)
		}
		defer removePidFile()
		
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
		pid, err := readPidFile()
		if err != nil {
			fmt.Println("❌ MemoryPilot daemon is not running (no PID file)")
			return nil
		}
		
		if !isProcessRunning(pid) {
			fmt.Println("❌ MemoryPilot daemon is not running (stale PID file)")
			removePidFile()
			return nil
		}
		
		fmt.Printf("🛑 Stopping MemoryPilot daemon (PID %d)...\n", pid)
		
		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("failed to find process: %w", err)
		}
		
		// Send SIGTERM for graceful shutdown
		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to stop daemon: %w", err)
		}
		
		fmt.Println("✅ MemoryPilot daemon stopped")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid, err := readPidFile()
		if err != nil {
			fmt.Println("🔴 MemoryPilot daemon is not running")
			return nil
		}
		
		if !isProcessRunning(pid) {
			fmt.Println("🔴 MemoryPilot daemon is not running (stale PID file)")
			removePidFile()
			return nil
		}
		
		fmt.Printf("🟢 MemoryPilot daemon is running (PID %d)\n", pid)
		fmt.Println()
		fmt.Println("Watched directories:")
		fmt.Println("  • ~/Documents/source-code/")
		fmt.Println("  • ~/Projects/")
		fmt.Println()
		fmt.Println("Watching:")
		fmt.Println("  • Git commits")
		fmt.Println("  • File changes")
		fmt.Println("  • Terminal commands")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	
	daemonStartCmd.Flags().BoolP("background", "b", false, "Run daemon in background")
}
