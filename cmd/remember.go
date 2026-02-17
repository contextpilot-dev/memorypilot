package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/contextpilot-dev/memorypilot/internal/store"
	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/oklog/ulid/v2"
	"github.com/spf13/cobra"
)

var rememberCmd = &cobra.Command{
	Use:   "remember [content]",
	Short: "Manually create a memory",
	Long: `Explicitly remember something important.

Examples:
  memorypilot remember "Always validate JWT tokens server-side"
  memorypilot remember --type decision "Chose PostgreSQL for ACID compliance"
  memorypilot remember --type mistake "Don't use float for currency"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content := strings.Join(args, " ")
		
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
		
		// Get flags
		memoryType, _ := cmd.Flags().GetString("type")
		topics, _ := cmd.Flags().GetStringSlice("topics")
		
		// Create memory
		now := time.Now()
		memory := models.Memory{
			ID:      ulid.Make().String(),
			Type:    models.MemoryType(memoryType),
			Content: content,
			Summary: truncate(content, 100),
			Scope:   models.MemoryScopePersonal,
			Source: models.Source{
				Type:      models.SourceTypeManual,
				Reference: "cli",
				Timestamp: now,
			},
			Confidence:     1.0, // Manual memories have full confidence
			Importance:     1.0,
			Topics:         topics,
			CreatedAt:      now,
			LastAccessedAt: now,
			AccessCount:    0,
		}
		
		// Save
		if err := s.CreateMemory(&memory); err != nil {
			return fmt.Errorf("failed to save memory: %w", err)
		}
		
		fmt.Printf("✅ Memory created: %s\n", memory.ID)
		fmt.Printf("   Type: %s\n", memory.Type)
		fmt.Printf("   %s\n", memory.Content)
		
		return nil
	},
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	rememberCmd.Flags().StringP("type", "t", "fact", "Memory type (decision|pattern|fact|preference|mistake|learning)")
	rememberCmd.Flags().StringSliceP("topics", "T", []string{}, "Topics/tags for this memory")
}
