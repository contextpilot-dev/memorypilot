package store

import (
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/contextpilot-dev/memorypilot/pkg/models"
)

// Store handles all database operations
type Store struct {
	db *sql.DB
}

// Stats represents store statistics
type Stats struct {
	TotalMemories int            `json:"totalMemories"`
	ByType        map[string]int `json:"byType"`
	ProjectCount  int            `json:"projectCount"`
	DaemonRunning bool           `json:"daemonRunning"`
}

// New creates a new store instance
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return s, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate runs database migrations
func (s *Store) migrate() error {
	migrations := []string{
		// Projects table
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			path TEXT UNIQUE NOT NULL,
			git_remote TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Memories table
		`CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL CHECK (type IN ('decision','pattern','fact','preference','mistake','learning')),
			content TEXT NOT NULL,
			summary TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'personal' CHECK (scope IN ('personal','project','team','org')),
			project_id TEXT REFERENCES projects(id),
			team_id TEXT,
			
			source_type TEXT NOT NULL,
			source_reference TEXT,
			source_timestamp DATETIME,
			
			confidence REAL NOT NULL DEFAULT 0.8,
			importance REAL NOT NULL DEFAULT 1.0,
			
			topics TEXT,
			related_memories TEXT,
			embedding BLOB,
			
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_accessed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			access_count INTEGER DEFAULT 0,
			expires_at DATETIME
		)`,

		// Events table
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			data TEXT,
			project_id TEXT REFERENCES projects(id),
			processed_at DATETIME
		)`,

		// Indexes
		`CREATE INDEX IF NOT EXISTS idx_memories_project ON memories(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories(importance DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC)`,
	}

	for _, migration := range migrations {
		if _, err := s.db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	return nil
}

// GetStats returns store statistics
func (s *Store) GetStats() (*Stats, error) {
	stats := &Stats{
		ByType: make(map[string]int),
	}

	// Total memories
	row := s.db.QueryRow("SELECT COUNT(*) FROM memories")
	if err := row.Scan(&stats.TotalMemories); err != nil {
		return nil, err
	}

	// By type
	rows, err := s.db.Query("SELECT type, COUNT(*) FROM memories GROUP BY type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var memType string
		var count int
		if err := rows.Scan(&memType, &count); err != nil {
			return nil, err
		}
		stats.ByType[memType] = count
	}

	// Project count
	row = s.db.QueryRow("SELECT COUNT(*) FROM projects")
	if err := row.Scan(&stats.ProjectCount); err != nil {
		return nil, err
	}

	return stats, nil
}

// CreateMemory stores a new memory
func (s *Store) CreateMemory(m *models.Memory) error {
	topicsJSON, _ := json.Marshal(m.Topics)
	relatedJSON, _ := json.Marshal(m.RelatedMemories)

	_, err := s.db.Exec(`
		INSERT INTO memories (
			id, type, content, summary, scope, project_id, team_id,
			source_type, source_reference, source_timestamp,
			confidence, importance, topics, related_memories, embedding,
			created_at, last_accessed_at, access_count, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		m.ID, m.Type, m.Content, m.Summary, m.Scope, m.ProjectID, m.TeamID,
		m.Source.Type, m.Source.Reference, m.Source.Timestamp,
		m.Confidence, m.Importance, string(topicsJSON), string(relatedJSON), nil,
		m.CreatedAt, m.LastAccessedAt, m.AccessCount, m.ExpiresAt,
	)

	return err
}

// Recall searches memories based on the request
func (s *Store) Recall(req models.RecallRequest) ([]models.Memory, error) {
	// Build query
	query := `
		SELECT id, type, content, summary, scope, project_id, team_id,
			   source_type, source_reference, source_timestamp,
			   confidence, importance, topics, related_memories,
			   created_at, last_accessed_at, access_count, expires_at
		FROM memories
		WHERE 1=1
	`
	args := []interface{}{}

	// Add filters
	if len(req.Scope) > 0 {
		placeholders := ""
		for i, scope := range req.Scope {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, scope)
		}
		query += " AND scope IN (" + placeholders + ")"
	}

	if len(req.Types) > 0 {
		placeholders := ""
		for i, t := range req.Types {
			if i > 0 {
				placeholders += ","
			}
			placeholders += "?"
			args = append(args, t)
		}
		query += " AND type IN (" + placeholders + ")"
	}

	if req.ProjectID != nil {
		query += " AND (project_id = ? OR project_id IS NULL)"
		args = append(args, *req.ProjectID)
	}

	// Text search (basic for now, will add vector search later)
	if req.Query != "" {
		query += " AND (content LIKE ? OR summary LIKE ? OR topics LIKE ?)"
		searchTerm := "%" + req.Query + "%"
		args = append(args, searchTerm, searchTerm, searchTerm)
	}

	// Order by importance and recency
	query += " ORDER BY importance DESC, last_accessed_at DESC"

	// Limit
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	query += " LIMIT ?"
	args = append(args, limit)

	// Execute
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []models.Memory
	for rows.Next() {
		var m models.Memory
		var topicsJSON, relatedJSON sql.NullString
		var projectID, teamID sql.NullString
		var expiresAt sql.NullTime

		err := rows.Scan(
			&m.ID, &m.Type, &m.Content, &m.Summary, &m.Scope, &projectID, &teamID,
			&m.Source.Type, &m.Source.Reference, &m.Source.Timestamp,
			&m.Confidence, &m.Importance, &topicsJSON, &relatedJSON,
			&m.CreatedAt, &m.LastAccessedAt, &m.AccessCount, &expiresAt,
		)
		if err != nil {
			return nil, err
		}

		if projectID.Valid {
			m.ProjectID = &projectID.String
		}
		if teamID.Valid {
			m.TeamID = &teamID.String
		}
		if expiresAt.Valid {
			m.ExpiresAt = &expiresAt.Time
		}
		if topicsJSON.Valid {
			json.Unmarshal([]byte(topicsJSON.String), &m.Topics)
		}
		if relatedJSON.Valid {
			json.Unmarshal([]byte(relatedJSON.String), &m.RelatedMemories)
		}

		memories = append(memories, m)

		// Record access
		s.recordAccess(m.ID)
	}

	return memories, nil
}

// recordAccess updates access statistics for a memory
func (s *Store) recordAccess(memoryID string) {
	s.db.Exec(`
		UPDATE memories
		SET last_accessed_at = ?,
			access_count = access_count + 1,
			importance = MIN(1.0, importance * 1.05)
		WHERE id = ?
	`, time.Now(), memoryID)
}

// DecayImportance reduces importance of old memories
func (s *Store) DecayImportance() error {
	_, err := s.db.Exec(`
		UPDATE memories
		SET importance = importance * 0.99
		WHERE importance > 0.1
		  AND last_accessed_at < datetime('now', '-1 day')
	`)
	return err
}

// CreateProject stores a new project
func (s *Store) CreateProject(p *models.Project) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO projects (id, name, path, git_remote, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.Path, p.GitRemote, p.CreatedAt, p.LastSeen)
	return err
}

// GetProjectByPath retrieves a project by its filesystem path
func (s *Store) GetProjectByPath(path string) (*models.Project, error) {
	row := s.db.QueryRow(`
		SELECT id, name, path, git_remote, created_at, last_seen
		FROM projects WHERE path = ?
	`, path)

	var p models.Project
	var gitRemote sql.NullString
	err := row.Scan(&p.ID, &p.Name, &p.Path, &gitRemote, &p.CreatedAt, &p.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if gitRemote.Valid {
		p.GitRemote = &gitRemote.String
	}
	return &p, nil
}

// CreateEvent stores a new event
func (s *Store) CreateEvent(e *models.Event) error {
	dataJSON, _ := json.Marshal(e.Data)
	_, err := s.db.Exec(`
		INSERT INTO events (id, type, timestamp, data, project_id)
		VALUES (?, ?, ?, ?, ?)
	`, e.ID, e.Type, e.Timestamp, string(dataJSON), e.ProjectID)
	return err
}

// GetUnprocessedEvents retrieves events that haven't been processed yet
func (s *Store) GetUnprocessedEvents(limit int) ([]models.Event, error) {
	rows, err := s.db.Query(`
		SELECT id, type, timestamp, data, project_id
		FROM events
		WHERE processed_at IS NULL
		ORDER BY timestamp ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var e models.Event
		var dataJSON sql.NullString
		var projectID sql.NullString

		err := rows.Scan(&e.ID, &e.Type, &e.Timestamp, &dataJSON, &projectID)
		if err != nil {
			return nil, err
		}

		if projectID.Valid {
			e.ProjectID = &projectID.String
		}
		if dataJSON.Valid {
			json.Unmarshal([]byte(dataJSON.String), &e.Data)
		}

		events = append(events, e)
	}

	return events, nil
}

// MarkEventProcessed marks an event as processed
func (s *Store) MarkEventProcessed(eventID string) error {
	_, err := s.db.Exec(`
		UPDATE events SET processed_at = ? WHERE id = ?
	`, time.Now(), eventID)
	return err
}

// UpdateMemoryEmbedding stores the embedding for a memory
func (s *Store) UpdateMemoryEmbedding(memoryID string, embedding []float32) error {
	blob := encodeEmbedding(embedding)
	_, err := s.db.Exec(`
		UPDATE memories SET embedding = ? WHERE id = ?
	`, blob, memoryID)
	return err
}

// SemanticSearch searches memories using vector similarity
func (s *Store) SemanticSearch(queryEmbedding []float32, limit int) ([]models.Memory, error) {
	// Get all memories with embeddings
	rows, err := s.db.Query(`
		SELECT id, type, content, summary, scope, project_id, team_id,
			   source_type, source_reference, source_timestamp,
			   confidence, importance, topics, related_memories, embedding,
			   created_at, last_accessed_at, access_count, expires_at
		FROM memories
		WHERE embedding IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scoredMemory struct {
		memory models.Memory
		score  float32
	}

	var scored []scoredMemory
	for rows.Next() {
		var m models.Memory
		var topicsJSON, relatedJSON sql.NullString
		var projectID, teamID sql.NullString
		var expiresAt sql.NullTime
		var embeddingBlob []byte

		err := rows.Scan(
			&m.ID, &m.Type, &m.Content, &m.Summary, &m.Scope, &projectID, &teamID,
			&m.Source.Type, &m.Source.Reference, &m.Source.Timestamp,
			&m.Confidence, &m.Importance, &topicsJSON, &relatedJSON, &embeddingBlob,
			&m.CreatedAt, &m.LastAccessedAt, &m.AccessCount, &expiresAt,
		)
		if err != nil {
			continue
		}

		if len(embeddingBlob) == 0 {
			continue
		}

		embedding := decodeEmbedding(embeddingBlob)
		similarity := cosineSimilarity(queryEmbedding, embedding)

		if projectID.Valid {
			m.ProjectID = &projectID.String
		}
		if teamID.Valid {
			m.TeamID = &teamID.String
		}
		if expiresAt.Valid {
			m.ExpiresAt = &expiresAt.Time
		}
		if topicsJSON.Valid {
			json.Unmarshal([]byte(topicsJSON.String), &m.Topics)
		}
		if relatedJSON.Valid {
			json.Unmarshal([]byte(relatedJSON.String), &m.RelatedMemories)
		}

		// Combine similarity with importance
		score := similarity*0.7 + float32(m.Importance)*0.3
		scored = append(scored, scoredMemory{memory: m, score: score})
	}

	// Sort by score (simple bubble sort for now, can optimize later)
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// Take top N
	var results []models.Memory
	for i := 0; i < len(scored) && i < limit; i++ {
		results = append(results, scored[i].memory)
		s.recordAccess(scored[i].memory.ID)
	}

	return results, nil
}

// HybridSearch combines semantic and keyword search
func (s *Store) HybridSearch(query string, queryEmbedding []float32, limit int) ([]models.Memory, error) {
	// Get semantic results
	var semanticResults []models.Memory
	if queryEmbedding != nil && len(queryEmbedding) > 0 {
		var err error
		semanticResults, err = s.SemanticSearch(queryEmbedding, limit*2)
		if err != nil {
			return nil, err
		}
	}

	// Get keyword results
	keywordResults, err := s.Recall(models.RecallRequest{
		Query: query,
		Limit: limit * 2,
	})
	if err != nil {
		return nil, err
	}

	// Merge results (semantic results first, then keyword results not in semantic)
	seen := make(map[string]bool)
	var merged []models.Memory

	for _, m := range semanticResults {
		if !seen[m.ID] {
			seen[m.ID] = true
			merged = append(merged, m)
		}
	}

	for _, m := range keywordResults {
		if !seen[m.ID] {
			seen[m.ID] = true
			merged = append(merged, m)
		}
	}

	// Limit results
	if len(merged) > limit {
		merged = merged[:limit]
	}

	return merged, nil
}

// Helper functions for embedding storage

func encodeEmbedding(embedding []float32) []byte {
	buf := make([]byte, len(embedding)*4)
	for i, v := range embedding {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func decodeEmbedding(data []byte) []float32 {
	embedding := make([]float32, len(data)/4)
	for i := range embedding {
		embedding[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return embedding
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
