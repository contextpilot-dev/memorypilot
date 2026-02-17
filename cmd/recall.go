package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/contextpilot-dev/memorypilot/internal/embedding"
	"github.com/contextpilot-dev/memorypilot/internal/store"
	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/spf13/cobra"
)

var recallCmd = &cobra.Command{
	Use:   "recall [query]",
	Short: "Search your memories",
	Long: `Search your memories using semantic search.

Examples:
  memorypilot recall "authentication patterns"
  memorypilot recall "how did we handle rate limiting"
  memorypilot recall --type decision "database choice"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		
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
		
		// Build recall request
		limit, _ := cmd.Flags().GetInt("limit")
		typeFilter, _ := cmd.Flags().GetString("type")
		scopeFilter, _ := cmd.Flags().GetStringSlice("scope")
		semantic, _ := cmd.Flags().GetBool("semantic")
		
		var memories []models.Memory
		
		if semantic {
			// Try semantic search with embeddings
			embedder := embedding.NewOllamaEmbedder("", "nomic-embed-text")
			queryEmb, err := embedder.Embed(query)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Semantic search unavailable (%v), falling back to keyword search\n", err)
				semantic = false
			} else {
				memories, err = s.HybridSearch(query, queryEmb, limit)
				if err != nil {
					return fmt.Errorf("hybrid search failed: %w", err)
				}
			}
		}
		
		if !semantic {
			// Keyword search
			req := models.RecallRequest{
				Query: query,
				Limit: limit,
			}
			
			if typeFilter != "" {
				req.Types = []models.MemoryType{models.MemoryType(typeFilter)}
			}
			
			if len(scopeFilter) > 0 {
				for _, sc := range scopeFilter {
					req.Scope = append(req.Scope, models.MemoryScope(sc))
				}
			}
			
			var err error
			memories, err = s.Recall(req)
			if err != nil {
				return fmt.Errorf("recall failed: %w", err)
			}
		}
		
		// Check if JSON output requested
		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, _ := json.MarshalIndent(memories, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		
		// Pretty print
		if len(memories) == 0 {
			fmt.Printf("🔍 No memories found for: %q\n", query)
			return nil
		}
		
		fmt.Printf("🧠 Found %d memories for: %q\n\n", len(memories), query)
		
		for i, m := range memories {
			typeEmoji := getTypeEmoji(m.Type)
			fmt.Printf("%s [%s] %s\n", typeEmoji, m.Type, m.Summary)
			fmt.Printf("   %s\n", m.Content)
			fmt.Printf("   📅 %s | 🎯 %.0f%% confidence\n", m.CreatedAt.Format("2006-01-02"), m.Confidence*100)
			if len(m.Topics) > 0 {
				fmt.Printf("   🏷️  %s\n", strings.Join(m.Topics, ", "))
			}
			if i < len(memories)-1 {
				fmt.Println()
			}
		}
		
		return nil
	},
}

func getTypeEmoji(t models.MemoryType) string {
	switch t {
	case models.MemoryTypeDecision:
		return "⚖️"
	case models.MemoryTypePattern:
		return "🔄"
	case models.MemoryTypeFact:
		return "📌"
	case models.MemoryTypePreference:
		return "❤️"
	case models.MemoryTypeMistake:
		return "⚠️"
	case models.MemoryTypeLearning:
		return "💡"
	default:
		return "📝"
	}
}

func init() {
	recallCmd.Flags().IntP("limit", "l", 5, "Maximum number of results")
	recallCmd.Flags().StringP("type", "t", "", "Filter by memory type (decision|pattern|fact|preference|mistake|learning)")
	recallCmd.Flags().StringSliceP("scope", "s", []string{}, "Filter by scope (personal|project|team)")
	recallCmd.Flags().Bool("json", false, "Output as JSON")
	recallCmd.Flags().BoolP("semantic", "S", true, "Use semantic search (requires Ollama)")
}
