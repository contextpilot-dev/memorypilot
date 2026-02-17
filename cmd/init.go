package cmd

import (
	"fmt"
	"os"

	"github.com/contextpilot-dev/memorypilot/internal/store"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize MemoryPilot",
	Long: `Initialize MemoryPilot in your home directory.

This creates:
  ~/.memorypilot/config.yaml    - Configuration file
  ~/.memorypilot/data/          - Database and embeddings
  ~/.memorypilot/logs/          - Log files`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configDir := getConfigDir()
		dataDir := getDataDir()
		logsDir := configDir + "/logs"
		
		fmt.Println("🧠 Initializing MemoryPilot...")
		
		// Create directories
		dirs := []string{configDir, dataDir, logsDir}
		for _, dir := range dirs {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
		}
		fmt.Println("   ✓ Created directories")
		
		// Create config file if it doesn't exist
		configPath := configDir + "/config.yaml"
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
				return fmt.Errorf("failed to create config: %w", err)
			}
			fmt.Println("   ✓ Created config.yaml")
		} else {
			fmt.Println("   ✓ Config exists")
		}
		
		// Initialize database
		dbPath := dataDir + "/memories.db"
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		s.Close()
		fmt.Println("   ✓ Initialized database")
		
		fmt.Println()
		fmt.Println("✅ MemoryPilot initialized!")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Start the daemon:  memorypilot daemon start")
		fmt.Println("  2. Check status:      memorypilot status")
		fmt.Println("  3. Search memories:   memorypilot recall \"your query\"")
		fmt.Println()
		fmt.Println("For MCP integration (Claude Code, OpenClaw):")
		fmt.Println("  Add to your MCP config:")
		fmt.Println(`  {`)
		fmt.Println(`    "mcpServers": {`)
		fmt.Println(`      "memorypilot": {`)
		fmt.Println(`        "command": "memorypilot",`)
		fmt.Println(`        "args": ["mcp"]`)
		fmt.Println(`      }`)
		fmt.Println(`    }`)
		fmt.Println(`  }`)
		
		return nil
	},
}

const defaultConfig = `# MemoryPilot Configuration

# LLM settings for memory extraction
extraction:
  provider: ollama  # ollama | claude
  model: llama3.2   # For ollama
  # apiKey: ""      # For claude (or set ANTHROPIC_API_KEY)

# Watcher settings
watchers:
  git:
    enabled: true
    interval: 30s
  file:
    enabled: true
    debounce: 500ms
    ignore:
      - node_modules
      - .git
      - dist
      - build
      - vendor
      - __pycache__
      - .venv
  terminal:
    enabled: true
    historyFiles:
      - ~/.zsh_history
      - ~/.bash_history

# API settings
api:
  port: 7832
  enabled: true

# Sync settings (Phase 2)
sync:
  enabled: false
  # endpoint: https://api.memorypilot.dev
`
