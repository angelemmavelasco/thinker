package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type LLMService struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewLLMService(baseURL, apiKey string) *LLMService {
	return &LLMService{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

type chatCompletionRequest struct {
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type IdeaAnalysisResult struct {
	RestructuredNote string   `json:"restructured_note"`
	KeyEntities      []string `json:"key_entities"`
}

func (s *LLMService) AnalyzeIdea(ctx context.Context, title, content string) (*IdeaAnalysisResult, error) {
	requestBody := chatCompletionRequest{
		Messages: []chatMessage{
			{
				Role: "system",
				Content: `Analiza la idea y responde únicamente con JSON válido, sin markdown, usando exactamente esta estructura:
{
  "restructured_note": "Una versión clara y organizada de la nota",
  "key_entities": ["Entidad o concepto clave"]
}
No inventes información que no aparezca en la nota.`,
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Título: %s\n\nContenido:\n%s", title, content),
			},
		},
		Temperature: 0.2,
		MaxTokens:   500,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("error serializando solicitud al LLM: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("error creando solicitud al LLM: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error conectando con el LLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("el LLM respondió con HTTP %d", resp.StatusCode)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("error leyendo respuesta del LLM: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("el LLM respondió sin resultados")
	}

	rawContent := strings.TrimSpace(completion.Choices[0].Message.Content)
	if strings.HasPrefix(rawContent, "```") {
		rawContent = strings.TrimPrefix(rawContent, "```json")
		rawContent = strings.TrimPrefix(rawContent, "```")
		rawContent = strings.TrimSuffix(rawContent, "```")
		rawContent = strings.TrimSpace(rawContent)
	}

	var result IdeaAnalysisResult
	if err := json.Unmarshal([]byte(rawContent), &result); err != nil {
		return nil, fmt.Errorf("error parseando JSON del LLM (%w). Respuesta cruda: %s", err, rawContent)
	}

	return &result, nil
}
