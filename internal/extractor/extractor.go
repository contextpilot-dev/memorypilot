package extractor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/contextpilot-dev/memorypilot/pkg/models"
)

// Extractor extracts memories from events using LLM
type Extractor interface {
	Extract(events []models.Event) ([]ExtractedMemory, error)
}

// ExtractedMemory represents a memory extracted by the LLM
type ExtractedMemory struct {
	Type       string   `json:"type"`
	Content    string   `json:"content"`
	Summary    string   `json:"summary"`
	Confidence float64  `json:"confidence"`
	Topics     []string `json:"topics"`
}

// OllamaExtractor uses Ollama for memory extraction
type OllamaExtractor struct {
	endpoint string
	model    string
	client   *http.Client
}

// NewOllamaExtractor creates a new Ollama-based extractor
func NewOllamaExtractor(endpoint, model string) *OllamaExtractor {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.2"
	}
	return &OllamaExtractor{
		endpoint: endpoint,
		model:    model,
		client: &http.Client{
			Timeout: 120 * time.Second, // LLM can be slow
		},
	}
}

const extractionPrompt = `You are a memory extraction system for a software developer.
Analyze the following development events and extract memories worth remembering.

For each memory, provide:
- type: One of: decision, pattern, fact, preference, mistake, learning
- content: The full memory (1-3 sentences, be specific)
- summary: Short version (under 80 characters)
- confidence: 0.0-1.0 how confident this is worth remembering
- topics: Array of relevant topics (2-5 keywords)

Rules:
- Only extract genuinely useful memories that would help an AI assistant
- Focus on: decisions made, patterns used, lessons learned, preferences shown
- Ignore: routine commits, trivial changes, boilerplate code
- Be specific: include WHY decisions were made if evident
- A batch of events might produce 0-3 memories (don't force it)

Events to analyze:
%s

Respond ONLY with valid JSON in this exact format (no markdown, no explanation):
{"memories": [{"type": "decision", "content": "...", "summary": "...", "confidence": 0.85, "topics": ["topic1", "topic2"]}]}

If no memories worth extracting, respond: {"memories": []}`

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Extract analyzes events and extracts memories
func (e *OllamaExtractor) Extract(events []models.Event) ([]ExtractedMemory, error) {
	if len(events) == 0 {
		return nil, nil
	}

	// Format events for the prompt
	eventsText := formatEvents(events)
	prompt := fmt.Sprintf(extractionPrompt, eventsText)

	req := ollamaGenerateRequest{
		Model:  e.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.Post(e.endpoint+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error: %s", string(body))
	}

	var result ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Parse the JSON response
	var extracted struct {
		Memories []ExtractedMemory `json:"memories"`
	}

	// Clean up response (sometimes LLM adds markdown)
	response := strings.TrimSpace(result.Response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	if err := json.Unmarshal([]byte(response), &extracted); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w (response: %s)", err, response)
	}

	// Filter by confidence
	var filtered []ExtractedMemory
	for _, m := range extracted.Memories {
		if m.Confidence >= 0.6 {
			filtered = append(filtered, m)
		}
	}

	return filtered, nil
}

func formatEvents(events []models.Event) string {
	var sb strings.Builder

	for i, e := range events {
		sb.WriteString(fmt.Sprintf("Event %d [%s] at %s:\n", i+1, e.Type, e.Timestamp.Format("2006-01-02 15:04")))

		switch e.Type {
		case "git_commit":
			if msg, ok := e.Data["message"].(string); ok {
				sb.WriteString(fmt.Sprintf("  Commit: %s\n", msg))
			}
			if files, ok := e.Data["files"].([]string); ok && len(files) > 0 {
				sb.WriteString(fmt.Sprintf("  Files: %s\n", strings.Join(files[:min(5, len(files))], ", ")))
			}
			if diff, ok := e.Data["diff"].(string); ok && len(diff) > 0 {
				// Truncate diff
				if len(diff) > 500 {
					diff = diff[:500] + "..."
				}
				sb.WriteString(fmt.Sprintf("  Diff summary: %s\n", diff))
			}

		case "file_change":
			if path, ok := e.Data["path"].(string); ok {
				sb.WriteString(fmt.Sprintf("  File: %s\n", path))
			}
			if content, ok := e.Data["content"].(string); ok && len(content) > 0 {
				// Truncate content
				if len(content) > 300 {
					content = content[:300] + "..."
				}
				sb.WriteString(fmt.Sprintf("  Content preview: %s\n", content))
			}

		case "terminal_cmd":
			if cmd, ok := e.Data["command"].(string); ok {
				sb.WriteString(fmt.Sprintf("  Command: %s\n", cmd))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NullExtractor is a no-op extractor for when LLM isn't available
type NullExtractor struct{}

func (e *NullExtractor) Extract(events []models.Event) ([]ExtractedMemory, error) {
	return nil, nil
}
