package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrompts_EmbeddedDefaults(t *testing.T) {
	mgr := NewManager("/non-existent-directory")

	analyze := mgr.AnalyzeIdea()
	if !strings.Contains(analyze, "restructured_note") || !strings.Contains(analyze, "key_entities") {
		t.Errorf("AnalyzeIdea prompt missing expected keys: %s", analyze)
	}

	expand := mgr.ExpandQuery()
	if !strings.Contains(expand, "recuperación semántica") || !strings.Contains(expand, "INSTRUCCIONES:") {
		t.Errorf("ExpandQuery prompt missing expected instructions: %s", expand)
	}

	ask := mgr.AskKnowledgeGraph()
	if !strings.Contains(ask, "Grounding estricto") || !strings.Contains(ask, "Neutralidad de interfaz") {
		t.Errorf("AskKnowledgeGraph prompt missing expected hardening rules: %s", ask)
	}
}

func TestPrompts_CustomDiskOverride(t *testing.T) {
	tempDir := t.TempDir()
	customPrompt := "Custom prompt override test from disk"
	err := os.WriteFile(filepath.Join(tempDir, FileAnalyzeIdea), []byte(customPrompt), 0644)
	if err != nil {
		t.Fatalf("failed to write custom prompt file: %v", err)
	}

	mgr := NewManager(tempDir)
	got := mgr.AnalyzeIdea()
	if got != customPrompt {
		t.Errorf("expected custom prompt %q, got %q", customPrompt, got)
	}

	// Test reload after modifying disk file
	updatedPrompt := "Updated custom prompt content"
	_ = os.WriteFile(filepath.Join(tempDir, FileAnalyzeIdea), []byte(updatedPrompt), 0644)
	mgr.Reload()
	gotUpdated := mgr.AnalyzeIdea()
	if gotUpdated != updatedPrompt {
		t.Errorf("expected updated prompt %q, got %q", updatedPrompt, gotUpdated)
	}
}
