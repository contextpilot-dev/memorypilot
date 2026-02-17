package watcher

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/oklog/ulid/v2"
)

// TerminalWatcher watches shell history for commands
type TerminalWatcher struct {
	eventSink     EventSink
	stopChan      chan struct{}
	historyFiles  []string
	lastPositions map[string]int64
}

// NewTerminalWatcher creates a new terminal watcher
func NewTerminalWatcher(sink EventSink) *TerminalWatcher {
	home, _ := os.UserHomeDir()
	return &TerminalWatcher{
		eventSink: sink,
		stopChan:  make(chan struct{}),
		historyFiles: []string{
			filepath.Join(home, ".zsh_history"),
			filepath.Join(home, ".bash_history"),
		},
		lastPositions: make(map[string]int64),
	}
}

// Start begins watching for terminal events
func (w *TerminalWatcher) Start() error {
	// Initialize positions
	for _, path := range w.historyFiles {
		if info, err := os.Stat(path); err == nil {
			w.lastPositions[path] = info.Size()
		}
	}

	go w.watch()
	return nil
}

// Stop stops the watcher
func (w *TerminalWatcher) Stop() {
	close(w.stopChan)
}

func (w *TerminalWatcher) watch() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.checkHistory()
		}
	}
}

func (w *TerminalWatcher) checkHistory() {
	for _, path := range w.historyFiles {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		lastPos := w.lastPositions[path]
		currentSize := info.Size()

		if currentSize <= lastPos {
			continue
		}

		// Read new lines
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		file.Seek(lastPos, 0)
		scanner := bufio.NewScanner(file)

		for scanner.Scan() {
			line := scanner.Text()
			cmd := w.parseHistoryLine(line, path)
			if cmd != "" && w.isInteresting(cmd) {
				w.emitEvent(cmd)
			}
		}

		file.Close()
		w.lastPositions[path] = currentSize
	}
}

func (w *TerminalWatcher) parseHistoryLine(line, historyFile string) string {
	// Handle zsh history format: : timestamp:0;command
	if strings.Contains(historyFile, "zsh") && strings.HasPrefix(line, ":") {
		parts := strings.SplitN(line, ";", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}
	return strings.TrimSpace(line)
}

func (w *TerminalWatcher) isInteresting(cmd string) bool {
	if len(cmd) < 3 {
		return false
	}

	// Skip sensitive commands
	sensitiveStarts := []string{
		"export ", "set ", "unset ",
		"curl ", "wget ", // May contain tokens
		"mysql ", "psql ", "redis-cli ",
		"ssh ", "scp ",
		"echo $", "cat ~/.",
	}

	for _, prefix := range sensitiveStarts {
		if strings.HasPrefix(cmd, prefix) {
			return false
		}
	}

	// Skip common noise
	noiseCommands := []string{
		"ls", "cd", "pwd", "clear", "exit",
		"history", "which", "whoami", "date",
	}

	parts := strings.Fields(cmd)
	if len(parts) > 0 {
		base := parts[0]
		for _, noise := range noiseCommands {
			if base == noise {
				return false
			}
		}
	}

	// Interesting commands
	interestingStarts := []string{
		"git ", "npm ", "yarn ", "pnpm ",
		"go ", "cargo ", "python ", "pip ",
		"docker ", "kubectl ", "terraform ",
		"make ", "brew ",
	}

	for _, prefix := range interestingStarts {
		if strings.HasPrefix(cmd, prefix) {
			return true
		}
	}

	return false
}

func (w *TerminalWatcher) emitEvent(cmd string) {
	event := models.Event{
		ID:        ulid.Make().String(),
		Type:      "terminal_cmd",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"command": cmd,
		},
	}

	log.Printf("Terminal event: %s", truncate(cmd, 50))

	select {
	case w.eventSink <- event:
	default:
		log.Printf("Event queue full, dropping terminal event")
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
