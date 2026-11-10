package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"thinker/prompts"
)

type LLMService struct {
	baseURL       string
	apiKey        string
	model         string
	httpClient    *http.Client
	client        *openai.Client
	promptManager *prompts.Manager
}

func NewLLMService(baseURL, apiKey string, model ...string) *LLMService {
	cleanBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleanBaseURL == "" {
		cleanBaseURL = "http://127.0.0.1:8081/v1"
	}
	if !strings.HasSuffix(cleanBaseURL, "/v1") {
		cleanBaseURL += "/v1"
	}

	m := "qwen2.5-3b-instruct"
	if len(model) > 0 && strings.TrimSpace(model[0]) != "" {
		m = strings.TrimSpace(model[0])
	}

	httpClient := &http.Client{
		Timeout: 2 * time.Minute,
	}

	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = cleanBaseURL
	cfg.HTTPClient = httpClient
	client := openai.NewClientWithConfig(cfg)

	return &LLMService{
		baseURL:       cleanBaseURL,
		apiKey:        apiKey,
		model:         m,
		httpClient:    httpClient,
		client:        client,
		promptManager: prompts.NewManager(),
	}
}

func (s *LLMService) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.httpClient = client
		cfg := openai.DefaultConfig(s.apiKey)
		cfg.BaseURL = s.baseURL
		cfg.HTTPClient = client
		s.client = openai.NewClientWithConfig(cfg)
	}
}

func (s *LLMService) SetPromptManager(pm *prompts.Manager) {
	if pm != nil {
		s.promptManager = pm
	}
}

func (s *LLMService) getPromptManager() *prompts.Manager {
	if s.promptManager != nil {
		return s.promptManager
	}
	return prompts.NewManager()
}

type chatCompletionRequest struct {
	Model       string        `json:"model,omitempty"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature"`
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
	systemPrompt := s.getPromptManager().AnalyzeIdea()
	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: s.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf("Título: %s\n\nContenido:\n%s", title, content),
			},
		},
		Temperature: 0.2,
		MaxTokens:   500,
	})
	if err != nil {
		return nil, fmt.Errorf("error conectando con el LLM: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("el LLM respondió sin resultados")
	}

	rawContent := cleanLLMJSON(resp.Choices[0].Message.Content)

	var result IdeaAnalysisResult
	if err := json.Unmarshal([]byte(rawContent), &result); err != nil {
		return nil, fmt.Errorf("error parseando JSON del LLM (%w). Respuesta cruda: %s", err, rawContent)
	}

	return &result, nil
}

func cleanLLMJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if start := strings.IndexByte(s, '{'); start != -1 {
		if end := strings.LastIndexByte(s, '}'); end != -1 && end > start {
			s = s[start : end+1]
		}
	}

	var out strings.Builder
	out.Grow(len(s))
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			out.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			out.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			out.WriteByte(ch)
			continue
		}
		if inString {
			switch ch {
			case '\n':
				out.WriteString(`\n`)
				continue
			case '\r':
				continue
			case '\t':
				out.WriteString(`\t`)
				continue
			}
		}
		out.WriteByte(ch)
	}

	return out.String()
}

const InsufficientContextResponse = "La documentación indexada no contiene información suficiente sobre este punto."

func (s *LLMService) ExpandQuery(ctx context.Context, query string) (string, error) {
	cleaned := strings.TrimSpace(query)
	if cleaned == "" {
		return "", fmt.Errorf("la consulta está vacía")
	}

	systemInstruction := s.getPromptManager().ExpandQuery()

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: s.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemInstruction,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf("Consulta:\n%s", cleaned),
			},
		},
		Temperature: 0.1,
		MaxTokens:   150,
	})
	if err != nil {
		return "", fmt.Errorf("error conectando con el LLM para expansión: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("el LLM de expansión respondió sin resultados")
	}

	expanded := strings.TrimSpace(resp.Choices[0].Message.Content)
	if expanded == "" {
		return cleaned, nil
	}

	return expanded, nil
}

func (s *LLMService) AskKnowledgeGraph(ctx context.Context, question string, subGraph *TraversalSubGraph) (string, error) {
	cleanedQuestion := strings.TrimSpace(question)
	if cleanedQuestion == "" {
		return "", fmt.Errorf("la pregunta está vacía")
	}

	if subGraph == nil || len(subGraph.Nodes) == 0 {
		return InsufficientContextResponse, nil
	}

	contextPrompt := BuildSubGraphPrompt(subGraph)
	if strings.TrimSpace(contextPrompt) == "" {
		return InsufficientContextResponse, nil
	}

	systemInstruction := s.getPromptManager().AskKnowledgeGraph()

	userContent := fmt.Sprintf("Pregunta:\n%s\n\nDocumentación de referencia:\n%s", cleanedQuestion, contextPrompt)

	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: s.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemInstruction,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userContent,
			},
		},
		Temperature: 0.1,
		MaxTokens:   800,
	})
	if err != nil {
		return "", fmt.Errorf("error conectando con el LLM: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("el LLM respondió sin resultados")
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
