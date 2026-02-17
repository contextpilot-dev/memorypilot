package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/contextpilot-dev/memorypilot/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show MemoryPilot status and statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDir := getDataDir()
		dbPath := dataDir + "/memories.db"
		
		// Check if database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("❌ MemoryPilot not initialized")
			fmt.Println("   Run 'memorypilot init' to get started")
			return nil
		}
		
		// Open store
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open store: %w", err)
		}
		defer s.Close()
		
		// Get stats
		stats, err := s.GetStats()
		if err != nil {
			return fmt.Errorf("failed to get stats: %w", err)
		}
		
		// Check if JSON output requested
		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		
		// Pretty print
		fmt.Println("🧠 MemoryPilot Status")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("   Version:    %s\n", version)
		fmt.Printf("   Status:     %s\n", getStatusEmoji(stats.DaemonRunning))
		fmt.Println()
		fmt.Println("📊 Memory Statistics")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("   Total:      %d\n", stats.TotalMemories)
		fmt.Printf("   Decisions:  %d\n", stats.ByType["decision"])
		fmt.Printf("   Patterns:   %d\n", stats.ByType["pattern"])
		fmt.Printf("   Facts:      %d\n", stats.ByType["fact"])
		fmt.Printf("   Preferences:%d\n", stats.ByType["preference"])
		fmt.Printf("   Mistakes:   %d\n", stats.ByType["mistake"])
		fmt.Printf("   Learnings:  %d\n", stats.ByType["learning"])
		fmt.Println()
		fmt.Println("📁 Projects")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("   Tracked:    %d\n", stats.ProjectCount)
		
		return nil
	},
}

func getStatusEmoji(running bool) string {
	if running {
		return "🟢 Running"
	}
	return "🔴 Stopped"
}

func init() {
	statusCmd.Flags().Bool("json", false, "Output as JSON")
}
