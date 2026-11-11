package prompts

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed analyze_idea.md
var defaultAnalyzeIdea string

//go:embed expand_query.md
var defaultExpandQuery string

//go:embed ask_knowledge_graph.md
var defaultAskKnowledgeGraph string

//go:embed triage_moc.md
var defaultTriageMOC string

const (
	FileAnalyzeIdea       = "analyze_idea.md"
	FileExpandQuery       = "expand_query.md"
	FileAskKnowledgeGraph = "ask_knowledge_graph.md"
	FileTriageMOC         = "triage_moc.md"
)

// Manager manages prompt retrieval from external documents with embedded fallbacks.
type Manager struct {
	mu         sync.RWMutex
	promptsDir string
	cache      map[string]string
}

// NewManager creates a prompt manager. If promptsDir is omitted, it defaults to "prompts" or PROMPTS_DIR.
func NewManager(promptsDir ...string) *Manager {
	dir := ""
	if len(promptsDir) > 0 && strings.TrimSpace(promptsDir[0]) != "" {
		dir = strings.TrimSpace(promptsDir[0])
	} else if envDir := strings.TrimSpace(os.Getenv("PROMPTS_DIR")); envDir != "" {
		dir = envDir
	} else {
		dir = "prompts"
	}

	return &Manager{
		promptsDir: dir,
		cache:      make(map[string]string),
	}
}

// GetPrompt retrieves a prompt from disk, falling back to the embedded default if unavailable.
func (m *Manager) GetPrompt(filename, embeddedFallback string) string {
	m.mu.RLock()
	if val, ok := m.cache[filename]; ok {
		m.mu.RUnlock()
		return val
	}
	m.mu.RUnlock()

	candidates := []string{
		filepath.Join(m.promptsDir, filename),
		filepath.Join(".", "prompts", filename),
		filepath.Join("..", "prompts", filename),
		filepath.Join("..", "..", "prompts", filename),
	}

	for _, path := range candidates {
		if content, err := os.ReadFile(path); err == nil {
			trimmed := strings.TrimSpace(string(content))
			if trimmed != "" {
				m.mu.Lock()
				m.cache[filename] = trimmed
				m.mu.Unlock()
				return trimmed
			}
		}
	}

	trimmedFallback := strings.TrimSpace(embeddedFallback)
	m.mu.Lock()
	m.cache[filename] = trimmedFallback
	m.mu.Unlock()
	return trimmedFallback
}

// Reload clears the cache so modified prompt files on disk are immediately reloaded.
func (m *Manager) Reload() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]string)
}

// AnalyzeIdea returns the prompt for note analysis and key entity extraction.
func (m *Manager) AnalyzeIdea() string {
	return m.GetPrompt(FileAnalyzeIdea, defaultAnalyzeIdea)
}

// ExpandQuery returns the prompt for query expansion and normalization.
func (m *Manager) ExpandQuery() string {
	return m.GetPrompt(FileExpandQuery, defaultExpandQuery)
}

// AskKnowledgeGraph returns the prompt for answering questions over the retrieved subgraph.
func (m *Manager) AskKnowledgeGraph() string {
	return m.GetPrompt(FileAskKnowledgeGraph, defaultAskKnowledgeGraph)
}

// TriageMOC returns the prompt for triaging queries into relevant MOC domains.
func (m *Manager) TriageMOC() string {
	return m.GetPrompt(FileTriageMOC, defaultTriageMOC)
}

var defaultManager = NewManager()

// SetDefaultDir sets the base prompt directory for the package-level default manager.
func SetDefaultDir(dir string) {
	defaultManager = NewManager(dir)
}

// Reload clears the package-level cache.
func Reload() {
	defaultManager.Reload()
}

// AnalyzeIdea returns the default prompt for idea analysis.
func AnalyzeIdea() string {
	return defaultManager.AnalyzeIdea()
}

// ExpandQuery returns the default prompt for query expansion.
func ExpandQuery() string {
	return defaultManager.ExpandQuery()
}

// AskKnowledgeGraph returns the default prompt for knowledge graph querying.
func AskKnowledgeGraph() string {
	return defaultManager.AskKnowledgeGraph()
}

// TriageMOC returns the default prompt for MOC domain triage.
func TriageMOC() string {
	return defaultManager.TriageMOC()
}
