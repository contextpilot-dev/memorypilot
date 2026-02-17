package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/contextpilot-dev/memorypilot/internal/embedding"
	"github.com/contextpilot-dev/memorypilot/internal/extractor"
	"github.com/contextpilot-dev/memorypilot/internal/store"
	"github.com/contextpilot-dev/memorypilot/internal/watcher"
	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/oklog/ulid/v2"
)

// Config holds agent configuration
type Config struct {
	DataDir         string
	GitInterval     time.Duration
	FileDebounce    time.Duration
	BatchSize       int
	BatchWait       time.Duration
	ExtractionModel string
}

// DefaultConfig returns the default agent configuration
func DefaultConfig() *Config {
	return &Config{
		GitInterval:     30 * time.Second,
		FileDebounce:    500 * time.Millisecond,
		BatchSize:       10,
		BatchWait:       5 * time.Second,
		ExtractionModel: "llama3.2",
	}
}

// Agent is the main MemoryPilot background service
type Agent struct {
	config     *Config
	store      *store.Store
	extractor  extractor.Extractor
	embedder   embedding.Embedder
	eventQueue chan models.Event
	watchers   []watcher.Watcher
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// New creates a new agent instance
func New(cfg *Config) (*Agent, error) {
	// Open store
	dbPath := cfg.DataDir + "/memories.db"
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}

	// Initialize extractor (Ollama)
	ext := extractor.NewOllamaExtractor("", cfg.ExtractionModel)

	// Initialize embedder (Ollama)
	emb := embedding.NewOllamaEmbedder("", "nomic-embed-text")

	ctx, cancel := context.WithCancel(context.Background())

	a := &Agent{
		config:     cfg,
		store:      s,
		extractor:  ext,
		embedder:   emb,
		eventQueue: make(chan models.Event, 10000),
		ctx:        ctx,
		cancel:     cancel,
	}

	return a, nil
}

// Start begins the agent's background processing
func (a *Agent) Start() error {
	log.Println("Starting MemoryPilot agent...")

	// Start event processor
	a.wg.Add(1)
	go a.processEvents()

	// Start watchers
	if err := a.startWatchers(); err != nil {
		return fmt.Errorf("failed to start watchers: %w", err)
	}

	// Start importance decay (daily)
	a.wg.Add(1)
	go a.decayLoop()

	log.Println("MemoryPilot agent started")
	return nil
}

// Stop gracefully shuts down the agent
func (a *Agent) Stop() {
	log.Println("Stopping MemoryPilot agent...")

	// Signal shutdown
	a.cancel()

	// Stop watchers
	for _, w := range a.watchers {
		w.Stop()
	}

	// Wait for goroutines
	a.wg.Wait()

	// Close store
	a.store.Close()

	log.Println("MemoryPilot agent stopped")
}

// startWatchers initializes and starts all watchers
func (a *Agent) startWatchers() error {
	// Git watcher
	gitWatcher := watcher.NewGitWatcher(a.config.GitInterval, a.eventQueue)
	if err := gitWatcher.Start(); err != nil {
		log.Printf("Warning: Git watcher failed to start: %v", err)
	} else {
		a.watchers = append(a.watchers, gitWatcher)
	}

	// File watcher
	fileWatcher := watcher.NewFileWatcher(a.config.FileDebounce, a.eventQueue)
	if err := fileWatcher.Start(); err != nil {
		log.Printf("Warning: File watcher failed to start: %v", err)
	} else {
		a.watchers = append(a.watchers, fileWatcher)
	}

	// Terminal watcher
	termWatcher := watcher.NewTerminalWatcher(a.eventQueue)
	if err := termWatcher.Start(); err != nil {
		log.Printf("Warning: Terminal watcher failed to start: %v", err)
	} else {
		a.watchers = append(a.watchers, termWatcher)
	}

	return nil
}

// processEvents handles the event queue
func (a *Agent) processEvents() {
	defer a.wg.Done()

	batch := make([]models.Event, 0, a.config.BatchSize)
	timer := time.NewTimer(a.config.BatchWait)

	for {
		select {
		case <-a.ctx.Done():
			// Process remaining batch
			if len(batch) > 0 {
				a.processBatch(batch)
			}
			return

		case event := <-a.eventQueue:
			// Store event
			if err := a.store.CreateEvent(&event); err != nil {
				log.Printf("Failed to store event: %v", err)
				continue
			}

			batch = append(batch, event)
			if len(batch) >= a.config.BatchSize {
				a.processBatch(batch)
				batch = batch[:0]
				timer.Reset(a.config.BatchWait)
			}

		case <-timer.C:
			if len(batch) > 0 {
				a.processBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(a.config.BatchWait)
		}
	}
}

// processBatch extracts memories from a batch of events
func (a *Agent) processBatch(events []models.Event) {
	log.Printf("Processing batch of %d events...", len(events))

	// Extract memories using LLM
	extracted, err := a.extractor.Extract(events)
	if err != nil {
		log.Printf("Extraction failed: %v", err)
		// Still mark events as processed to avoid reprocessing
		for _, e := range events {
			a.store.MarkEventProcessed(e.ID)
		}
		return
	}

	log.Printf("Extracted %d memories from batch", len(extracted))

	// Create memories in store
	for _, ext := range extracted {
		now := time.Now()
		memory := models.Memory{
			ID:      ulid.Make().String(),
			Type:    models.MemoryType(ext.Type),
			Content: ext.Content,
			Summary: ext.Summary,
			Scope:   models.MemoryScopePersonal,
			Source: models.Source{
				Type:      models.SourceTypeGit, // Default, could be smarter
				Reference: "batch",
				Timestamp: now,
			},
			Confidence:     ext.Confidence,
			Importance:     1.0,
			Topics:         ext.Topics,
			CreatedAt:      now,
			LastAccessedAt: now,
			AccessCount:    0,
		}

		// Save memory
		if err := a.store.CreateMemory(&memory); err != nil {
			log.Printf("Failed to save memory: %v", err)
			continue
		}

		// Generate and store embedding
		emb, err := a.embedder.Embed(memory.Content)
		if err != nil {
			log.Printf("Failed to generate embedding: %v", err)
		} else if emb != nil {
			if err := a.store.UpdateMemoryEmbedding(memory.ID, emb); err != nil {
				log.Printf("Failed to store embedding: %v", err)
			}
		}

		log.Printf("Created memory: [%s] %s", memory.Type, memory.Summary)
	}

	// Mark events as processed
	for _, e := range events {
		if err := a.store.MarkEventProcessed(e.ID); err != nil {
			log.Printf("Failed to mark event processed: %v", err)
		}
	}

	log.Printf("Batch processed")
}

// decayLoop periodically decays memory importance
func (a *Agent) decayLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if err := a.store.DecayImportance(); err != nil {
				log.Printf("Failed to decay importance: %v", err)
			}
		}
	}
}
