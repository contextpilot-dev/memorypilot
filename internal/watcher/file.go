package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/oklog/ulid/v2"
)

// FileWatcher watches for file system changes
type FileWatcher struct {
	debounce   time.Duration
	eventSink  EventSink
	watcher    *fsnotify.Watcher
	stopChan   chan struct{}
	pending    map[string]time.Time
	pendingMux sync.Mutex
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(debounce time.Duration, sink EventSink) *FileWatcher {
	return &FileWatcher{
		debounce:  debounce,
		eventSink: sink,
		stopChan:  make(chan struct{}),
		pending:   make(map[string]time.Time),
	}
}

// Start begins watching for file events
func (w *FileWatcher) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	go w.watch()
	go w.debounceLoop()

	// Add common code directories
	home, _ := os.UserHomeDir()
	codeDirs := []string{
		filepath.Join(home, "Documents", "source-code"),
		filepath.Join(home, "Projects"),
	}

	for _, dir := range codeDirs {
		w.addDirRecursive(dir)
	}

	return nil
}

// Stop stops the watcher
func (w *FileWatcher) Stop() {
	close(w.stopChan)
	if w.watcher != nil {
		w.watcher.Close()
	}
}

func (w *FileWatcher) addDirRecursive(root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip ignored directories
		if info.IsDir() {
			name := info.Name()
			if w.shouldIgnore(name) {
				return filepath.SkipDir
			}

			// Limit depth
			depth := strings.Count(strings.TrimPrefix(path, root), string(os.PathSeparator))
			if depth > 4 {
				return filepath.SkipDir
			}

			w.watcher.Add(path)
		}

		return nil
	})
}

func (w *FileWatcher) shouldIgnore(name string) bool {
	ignoreList := []string{
		"node_modules",
		".git",
		"dist",
		"build",
		"vendor",
		"__pycache__",
		".venv",
		"venv",
		".next",
		".nuxt",
		"target",
		"coverage",
		".cache",
	}

	for _, ignore := range ignoreList {
		if name == ignore {
			return true
		}
	}
	return false
}

func (w *FileWatcher) watch() {
	for {
		select {
		case <-w.stopChan:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Filter events
			if !w.isInteresting(event) {
				continue
			}

			// Debounce
			w.pendingMux.Lock()
			w.pending[event.Name] = time.Now()
			w.pendingMux.Unlock()

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)
		}
	}
}

func (w *FileWatcher) debounceLoop() {
	ticker := time.NewTicker(w.debounce)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			return
		case <-ticker.C:
			w.flushPending()
		}
	}
}

func (w *FileWatcher) flushPending() {
	w.pendingMux.Lock()
	defer w.pendingMux.Unlock()

	now := time.Now()
	for path, lastSeen := range w.pending {
		if now.Sub(lastSeen) >= w.debounce {
			w.emitEvent(path)
			delete(w.pending, path)
		}
	}
}

func (w *FileWatcher) isInteresting(event fsnotify.Event) bool {
	// Only care about writes and creates
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	name := filepath.Base(event.Name)
	ext := filepath.Ext(event.Name)

	// Interesting file types
	interestingExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".rs": true, ".java": true, ".kt": true, ".swift": true, ".c": true, ".cpp": true,
		".yaml": true, ".yml": true, ".json": true, ".toml": true,
		".md": true, ".sql": true, ".graphql": true,
		".dockerfile": true, ".env": true,
	}

	// Interesting file names
	interestingNames := map[string]bool{
		"Makefile": true, "Dockerfile": true, "docker-compose.yml": true,
		"package.json": true, "go.mod": true, "requirements.txt": true,
		"Cargo.toml": true, "pom.xml": true, "build.gradle": true,
	}

	return interestingExts[ext] || interestingNames[name]
}

func (w *FileWatcher) emitEvent(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	// Read content for small files
	var content string
	if info.Size() < 10000 {
		data, err := os.ReadFile(path)
		if err == nil {
			content = string(data)
		}
	}

	event := models.Event{
		ID:        ulid.Make().String(),
		Type:      "file_change",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"path":     path,
			"filename": filepath.Base(path),
			"ext":      filepath.Ext(path),
			"size":     info.Size(),
			"content":  content,
		},
	}

	log.Printf("File event: %s", filepath.Base(path))

	select {
	case w.eventSink <- event:
	default:
		log.Printf("Event queue full, dropping file event")
	}
}
