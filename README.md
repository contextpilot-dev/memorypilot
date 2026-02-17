# MemoryPilot

> One memory. Every AI. Zero repetition.

MemoryPilot is a **passive, intelligent memory layer** for AI-assisted development. It automatically captures context from your work (git commits, file changes, terminal commands) and makes it available to any AI tool.

**Your AI tools will finally remember you.**

## The Problem

```
You explain your project    → ChatGPT     → Session ends → FORGOTTEN
You explain it again        → Claude Code → Session ends → FORGOTTEN  
You explain it again        → Cursor      → Session ends → FORGOTTEN
You explain it again        → Gemini      → Session ends → FORGOTTEN

Same context. Explained 4 times. Every. Single. Day.
```

## The Solution

MemoryPilot runs in the background, watching your development activity and building a persistent memory that follows you across **all** AI tools.

```
                    MemoryPilot
                   (One Memory)
                        │
        ┌───────┬───────┼───────┬───────┐
        ▼       ▼       ▼       ▼       ▼
    ChatGPT  Claude  Cursor  Gemini  Windsurf
```

## Installation

```bash
# macOS/Linux
curl -fsSL https://contextpilot.dev/memorypilot/install.sh | sh

# Go install
go install github.com/memorypilot/memorypilot@latest

# From source
git clone https://github.com/memorypilot/memorypilot.git
cd memorypilot
go build -o memorypilot .
```

## Quick Start

```bash
# Initialize MemoryPilot
memorypilot init

# Start the background daemon
memorypilot daemon start

# Check status
memorypilot status

# Search your memories
memorypilot recall "authentication patterns"

# Manually remember something
memorypilot remember --type decision "Chose PostgreSQL for ACID compliance"
```

## MCP Integration (Claude Code, OpenClaw, Windsurf)

Add to your MCP configuration:

```json
{
  "mcpServers": {
    "memorypilot": {
      "command": "memorypilot",
      "args": ["mcp"]
    }
  }
}
```

Then in your AI chat:

```
You: "How did we handle auth before?"
AI: [calls memorypilot_recall]
AI: "Based on your memory, you implemented OAuth2 with PKCE 
     for the mobile app on January 15th..."
```

## Features

### What MemoryPilot Captures

| Source | What We Learn |
|--------|---------------|
| **Git commits** | Decisions, patterns, history |
| **File changes** | Architecture evolution, refactors |
| **Terminal commands** | Workflows, tools, processes |

### Memory Types

| Type | Description |
|------|-------------|
| `decision` | Architectural or technical choices |
| `pattern` | Recurring approaches or solutions |
| `fact` | Objective information |
| `preference` | Personal or team preferences |
| `mistake` | Errors to avoid |
| `learning` | New knowledge acquired |

### Privacy First

- **Local-first**: All data stored locally by default
- **Smart filtering**: Automatically redacts secrets and sensitive data
- **No telemetry**: Your memory is yours

## Commands

```bash
memorypilot init          # Initialize MemoryPilot
memorypilot daemon start  # Start background daemon
memorypilot daemon stop   # Stop background daemon
memorypilot status        # Show status and statistics
memorypilot recall        # Search memories
memorypilot remember      # Manually create a memory
memorypilot mcp           # Start MCP server (for AI tool integration)
```

## Configuration

Configuration file: `~/.memorypilot/config.yaml`

```yaml
# LLM for memory extraction
extraction:
  provider: ollama  # ollama | claude
  model: llama3.2

# Watchers
watchers:
  git:
    enabled: true
    interval: 30s
  file:
    enabled: true
    ignore: [node_modules, .git, dist]
  terminal:
    enabled: true
```

## Roadmap

- [x] Core agent with watchers
- [x] SQLite memory store
- [x] CLI commands
- [x] MCP server
- [ ] Vector embeddings (semantic search)
- [ ] AI-powered memory extraction
- [ ] VS Code extension
- [ ] Web dashboard
- [ ] Cloud sync
- [ ] Team features

## License

MIT

---

Built with ❤️ by [Jitin Gambhir](https://github.com/jitin-nhz)
