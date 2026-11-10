package config

import (
	"os"
	"testing"
)

func TestLoadConfig_ExternalProviders(t *testing.T) {
	// Set test environment variables
	os.Setenv("DEBUG", "true")
	os.Setenv("EXTERNAL_LLM_PROVIDER", "true")
	os.Setenv("EXTERNAL_EMBEDDING_PROVIDER", "true")
	os.Setenv("LLM_BASE_URL", "https://api.example.com/v1")
	os.Setenv("LLM_API_KEY", "test-external-key")
	os.Setenv("LLM_MODEL", "custom-llm-model")
	os.Setenv("EMBEDDING_BASE_URL", "https://api.example.com/v1")
	os.Setenv("EMBEDDING_API_KEY", "test-external-embed-key")
	os.Setenv("EMBEDDING_MODEL", "custom-embedding-model")
	defer func() {
		os.Unsetenv("EXTERNAL_LLM_PROVIDER")
		os.Unsetenv("EXTERNAL_EMBEDDING_PROVIDER")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("LLM_MODEL")
		os.Unsetenv("EMBEDDING_BASE_URL")
		os.Unsetenv("EMBEDDING_API_KEY")
		os.Unsetenv("EMBEDDING_MODEL")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !cfg.ExternalLLMProvider {
		t.Errorf("expected ExternalLLMProvider to be true")
	}
	if !cfg.ExternalEmbeddingProvider {
		t.Errorf("expected ExternalEmbeddingProvider to be true")
	}
	if cfg.LLMBaseURL != "https://api.example.com/v1" {
		t.Errorf("expected LLMBaseURL 'https://api.example.com/v1', got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMApiKey != "test-external-key" {
		t.Errorf("expected LLMApiKey 'test-external-key', got %q", cfg.LLMApiKey)
	}
	if cfg.LLMModel != "custom-llm-model" {
		t.Errorf("expected LLMModel 'custom-llm-model', got %q", cfg.LLMModel)
	}
	if cfg.EmbeddingBaseURL != "https://api.example.com/v1" {
		t.Errorf("expected EmbeddingBaseURL 'https://api.example.com/v1', got %q", cfg.EmbeddingBaseURL)
	}
	if cfg.EmbeddingApiKey != "test-external-embed-key" {
		t.Errorf("expected EmbeddingApiKey 'test-external-embed-key', got %q", cfg.EmbeddingApiKey)
	}
	if cfg.EmbeddingModel != "custom-embedding-model" {
		t.Errorf("expected EmbeddingModel 'custom-embedding-model', got %q", cfg.EmbeddingModel)
	}
}

func TestLoadConfig_LocalProviderDefaults(t *testing.T) {
	os.Setenv("DEBUG", "true")
	os.Setenv("EXTERNAL_LLM_PROVIDER", "false")
	os.Setenv("EXTERNAL_EMBEDDING_PROVIDER", "false")
	os.Unsetenv("LLM_BASE_URL")
	os.Unsetenv("LLM_API_KEY")
	os.Unsetenv("LLM_MODEL")
	os.Unsetenv("EMBEDDING_BASE_URL")
	os.Unsetenv("EMBEDDING_API_KEY")
	os.Unsetenv("EMBEDDING_MODEL")
	defer func() {
		os.Unsetenv("EXTERNAL_LLM_PROVIDER")
		os.Unsetenv("EXTERNAL_EMBEDDING_PROVIDER")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ExternalLLMProvider {
		t.Errorf("expected ExternalLLMProvider to be false")
	}
	if cfg.ExternalEmbeddingProvider {
		t.Errorf("expected ExternalEmbeddingProvider to be false")
	}
	if cfg.LLMBaseURL != "http://127.0.0.1:8081/v1" {
		t.Errorf("expected local default LLMBaseURL 'http://127.0.0.1:8081/v1', got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "qwen2.5-3b-instruct" {
		t.Errorf("expected local default LLMModel 'qwen2.5-3b-instruct', got %q", cfg.LLMModel)
	}
	if cfg.EmbeddingBaseURL != "http://127.0.0.1:8082/v1" {
		t.Errorf("expected local default EmbeddingBaseURL 'http://127.0.0.1:8082/v1', got %q", cfg.EmbeddingBaseURL)
	}
	if cfg.EmbeddingModel != "bge-small-en-v1.5" {
		t.Errorf("expected local default EmbeddingModel 'bge-small-en-v1.5', got %q", cfg.EmbeddingModel)
	}
}

func TestLoadConfig_Hybrid_ExternalLLM_LocalEmbeddings(t *testing.T) {
	os.Setenv("DEBUG", "true")
	os.Setenv("EXTERNAL_LLM_PROVIDER", "true")
	os.Setenv("EXTERNAL_EMBEDDING_PROVIDER", "false")
	os.Setenv("LLM_BASE_URL", "https://api.example.com/v1")
	os.Setenv("LLM_API_KEY", "test-external-llm-key")
	os.Setenv("LLM_MODEL", "custom-llm-model")
	os.Setenv("EMBEDDING_BASE_URL", "https://api.example.com/v1")
	os.Setenv("EMBEDDING_API_KEY", "test-external-embed-key")
	os.Setenv("EMBEDDING_MODEL", "custom-embed-model")
	defer func() {
		os.Unsetenv("EXTERNAL_LLM_PROVIDER")
		os.Unsetenv("EXTERNAL_EMBEDDING_PROVIDER")
		os.Unsetenv("LLM_BASE_URL")
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("LLM_MODEL")
		os.Unsetenv("EMBEDDING_BASE_URL")
		os.Unsetenv("EMBEDDING_API_KEY")
		os.Unsetenv("EMBEDDING_MODEL")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !cfg.ExternalLLMProvider {
		t.Errorf("expected ExternalLLMProvider to be true")
	}
	if cfg.ExternalEmbeddingProvider {
		t.Errorf("expected ExternalEmbeddingProvider to be false")
	}
	if cfg.LLMBaseURL != "https://api.example.com/v1" {
		t.Errorf("expected LLMBaseURL 'https://api.example.com/v1', got %q", cfg.LLMBaseURL)
	}
	if cfg.LLMModel != "custom-llm-model" {
		t.Errorf("expected LLMModel 'custom-llm-model', got %q", cfg.LLMModel)
	}

	if cfg.EmbeddingBaseURL != "http://127.0.0.1:8082/v1" {
		t.Errorf("expected local EmbeddingBaseURL 'http://127.0.0.1:8082/v1', got %q", cfg.EmbeddingBaseURL)
	}
	if cfg.EmbeddingModel != "bge-small-en-v1.5" {
		t.Errorf("expected local EmbeddingModel 'bge-small-en-v1.5', got %q", cfg.EmbeddingModel)
	}
	if cfg.EmbeddingApiKey != "" {
		t.Errorf("expected empty EmbeddingApiKey for local provider, got %q", cfg.EmbeddingApiKey)
	}
}
