package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type EmbeddingService struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	client     *openai.Client
}

func NewEmbeddingService(baseURL, apiKey string, model ...string) *EmbeddingService {
	cleanBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleanBaseURL == "" {
		cleanBaseURL = "http://127.0.0.1:8082/v1"
	}
	if !strings.HasSuffix(cleanBaseURL, "/v1") {
		cleanBaseURL += "/v1"
	}

	m := "bge-small-en-v1.5"
	if len(model) > 0 && strings.TrimSpace(model[0]) != "" {
		m = strings.TrimSpace(model[0])
	}

	httpClient := &http.Client{
		Timeout: 1 * time.Minute,
	}

	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = cleanBaseURL
	cfg.HTTPClient = httpClient
	client := openai.NewClientWithConfig(cfg)

	return &EmbeddingService{
		baseURL:    cleanBaseURL,
		apiKey:     apiKey,
		model:      m,
		httpClient: httpClient,
		client:     client,
	}
}

func (s *EmbeddingService) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.httpClient = client
		cfg := openai.DefaultConfig(s.apiKey)
		cfg.BaseURL = s.baseURL
		cfg.HTTPClient = client
		s.client = openai.NewClientWithConfig(cfg)
	}
}

type embeddingRequest struct {
	Model string   `json:"model,omitempty"`
	Input any      `json:"input"`
	Texts []string `json:"texts,omitempty"`
	Type  string   `json:"type,omitempty"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Embedding []float64   `json:"embedding"`
	Vectors   [][]float64 `json:"vectors"`
	BaseResp  *struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp,omitempty"`
}

func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float64, error) {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return nil, fmt.Errorf("el texto para embedding está vacío")
	}
	runes := []rune(cleaned)
	if len(runes) > 1200 {
		cleaned = string(runes[:1200])
	}

	maxAttempts := 5
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		vec, err := s.doGenerateEmbedding(ctx, cleaned)
		if err == nil {
			return vec, nil
		}
		lastErr = err

		errStr := err.Error()
		isRateLimit := strings.Contains(errStr, "1002") || strings.Contains(errStr, "429") || strings.Contains(errStr, "rate limit")
		if !isRateLimit || attempt == maxAttempts {
			break
		}

		backoff := time.Duration(attempt*2000) * time.Millisecond
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return nil, lastErr
}

func (s *EmbeddingService) doGenerateEmbedding(ctx context.Context, cleaned string) ([]float64, error) {
	payload := embeddingRequest{
		Model: s.model,
		Input: []string{cleaned},
		Texts: []string{cleaned},
		Type:  "db",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud de embedding: %w", err)
	}

	endpoint := s.baseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("error creando solicitud de embedding: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error conectando con el servidor de embeddings: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta de embeddings: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("el servidor de embeddings respondió con HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed openAIEmbeddingResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("error decodificando respuesta de embeddings: %w", err)
	}

	if parsed.BaseResp != nil && parsed.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("error del proveedor de embeddings (%d): %s", parsed.BaseResp.StatusCode, parsed.BaseResp.StatusMsg)
	}

	if len(parsed.Vectors) > 0 && len(parsed.Vectors[0]) > 0 {
		return parsed.Vectors[0], nil
	}

	if len(parsed.Data) > 0 && len(parsed.Data[0].Embedding) > 0 {
		return parsed.Data[0].Embedding, nil
	}

	if len(parsed.Embedding) > 0 {
		return parsed.Embedding, nil
	}

	return nil, fmt.Errorf("el servidor de embeddings devolvió un vector vacío")
}

func SerializeVector(vec []float64) (string, error) {
	if len(vec) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(vec)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func DeserializeVector(raw string) ([]float64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var vec []float64
	if err := json.Unmarshal([]byte(trimmed), &vec); err != nil {
		return nil, err
	}
	return vec, nil
}

func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return dot / denom
}
