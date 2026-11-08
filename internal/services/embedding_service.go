package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

type EmbeddingService struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewEmbeddingService(baseURL, apiKey string) *EmbeddingService {
	return &EmbeddingService{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 1 * time.Minute,
		},
	}
}

type embeddingRequest struct {
	Input string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Embedding []float64 `json:"embedding"`
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

	body, err := json.Marshal(embeddingRequest{Input: cleaned})
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud de embedding: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/v1/embeddings",
		bytes.NewReader(body),
	)
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

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("el servidor de embeddings respondió con HTTP %d", resp.StatusCode)
	}

	var parsed openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("error decodificando respuesta de embeddings: %w", err)
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
