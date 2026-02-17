package watcher

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/oklog/ulid/v2"
)

// GitWatcher watches git repositories for new commits
type GitWatcher struct {
	interval   time.Duration
	eventSink  EventSink
	stopChan   chan struct{}
	lastCommit map[string]string // repo path -> last commit hash
}

// NewGitWatcher creates a new git watcher
func NewGitWatcher(interval time.Duration, sink EventSink) *GitWatcher {
	return &GitWatcher{
		interval:   interval,
		eventSink:  sink,
		stopChan:   make(chan struct{}),
		lastCommit: make(map[string]string),
	}
}

// Start begins watching for git events
func (w *GitWatcher) Start() error {
	go w.watch()
	return nil
}

// Stop stops the watcher
func (w *GitWatcher) Stop() {
	close(w.stopChan)
}

func (w *GitWatcher) watch() {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Initial scan
	w.scanGitRepos()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.scanGitRepos()
		}
	}
}

func (w *GitWatcher) scanGitRepos() {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Common code directories
	codeDirs := []string{
		filepath.Join(home, "Documents", "source-code"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "code"),
		filepath.Join(home, "dev"),
	}

	for _, codeDir := range codeDirs {
		if _, err := os.Stat(codeDir); os.IsNotExist(err) {
			continue
		}

		// Find git repos
		filepath.Walk(codeDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}

			// Skip deep directories
			depth := strings.Count(strings.TrimPrefix(path, codeDir), string(os.PathSeparator))
			if depth > 3 {
				return filepath.SkipDir
			}

			// Check for .git directory
			if info.IsDir() && info.Name() == ".git" {
				repoPath := filepath.Dir(path)
				w.checkRepo(repoPath)
				return filepath.SkipDir
			}

			return nil
		})
	}
}

func (w *GitWatcher) checkRepo(repoPath string) {
	// Get latest commit
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--format=%H|%s|%an|%ae|%ai")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 5)
	if len(parts) < 5 {
		return
	}

	hash := parts[0]
	message := parts[1]
	author := parts[2]
	// email := parts[3]
	// dateStr := parts[4]

	// Check if this is a new commit
	lastHash, seen := w.lastCommit[repoPath]
	if seen && lastHash == hash {
		return
	}

	w.lastCommit[repoPath] = hash

	// Skip if this is the first time we're seeing this repo
	if !seen {
		return
	}

	// Get diff stats
	diffCmd := exec.Command("git", "-C", repoPath, "diff", "--stat", lastHash+".."+hash)
	diffOutput, _ := diffCmd.Output()

	// Get changed files
	filesCmd := exec.Command("git", "-C", repoPath, "diff", "--name-only", lastHash+".."+hash)
	filesOutput, _ := filesCmd.Output()

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(filesOutput)))
	for scanner.Scan() {
		files = append(files, scanner.Text())
	}

	// Create event
	event := models.Event{
		ID:        ulid.Make().String(),
		Type:      "git_commit",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"repo":    repoPath,
			"hash":    hash,
			"message": message,
			"author":  author,
			"diff":    string(diffOutput),
			"files":   files,
		},
	}

	log.Printf("Git event: %s - %s", filepath.Base(repoPath), message)

	select {
	case w.eventSink <- event:
	default:
		log.Printf("Event queue full, dropping git event")
	}
}
