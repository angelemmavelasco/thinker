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

func (s *LLMService) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.httpClient = client
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
				Content: `Analiza la nota y responde únicamente con JSON válido, sin markdown, usando esta estructura:
{
  "restructured_note": "Síntesis clara y organizada de la nota en máximo 3 oraciones",
  "key_entities": ["Concepto clave 1", "Concepto clave 2"]
}
Extrae entre 3 y 7 conceptos clave cortos. No inventes información.`,
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

	rawContent := cleanLLMJSON(completion.Choices[0].Message.Content)

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

	systemInstruction := "Eres un asistente especializado en normalización y expansión de consultas para recuperación semántica (RAG). " +
		"Tu objetivo es transformar la consulta del usuario en una formulación formal, técnica y descriptiva para maximizar la precisión de búsqueda semántica y léxica en la base de datos documental.\n\n" +
		"INSTRUCCIONES:\n" +
		"1. Traduce modismos, jergas, abreviaturas o lenguaje coloquial a términos formales, regulatorios o institucionales equivalentes.\n" +
		"2. Identifica la intención principal y desglosa los conceptos y sinónimos relevantes sin alterar el objetivo de la pregunta.\n" +
		"3. Responde ÚNICAMENTE con la consulta normalizada y términos clave en una o dos oraciones directas, sin explicaciones, prefijos ni introducciones."

	requestBody := chatCompletionRequest{
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: systemInstruction,
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Consulta:\n%s", cleaned),
			},
		},
		Temperature: 0.1,
		MaxTokens:   150,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error serializando solicitud de expansión: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("error creando solicitud de expansión: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error conectando con el LLM para expansión: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("el LLM de expansión respondió con HTTP %d", resp.StatusCode)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return "", fmt.Errorf("error leyendo respuesta de expansión: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("el LLM de expansión respondió sin resultados")
	}

	expanded := strings.TrimSpace(completion.Choices[0].Message.Content)
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

	systemInstruction := "Actúa como un asistente documental preciso, profesional y directo. " +
		"Tu función es responder a la pregunta del usuario basándote EXCLUSIVAMENTE en la información de la documentación provista.\n\n" +
		"REGLAS OBLIGATORIAS:\n" +
		"1. Grounding estricto: Responde única y exclusivamente con base en los hechos explícitos provistos en la documentación. " +
		"Si la documentación provista no contiene información suficiente para responder a la pregunta con certeza o no trata el tema consultado, debes responder exactamente:\n" +
		"\"La documentación indexada no contiene información suficiente sobre este punto.\"\n" +
		"Queda terminantemente prohibido inventar información, suponer, conjeturar o sugerir alternativas externas (como acudir a abogados particulares, cambiar de institución o ciudad, o sugerencias de sentido común no respaldadas por los textos).\n" +
		"2. Neutralidad de interfaz: No menciones jamás la arquitectura técnica interna del sistema ni cómo se almacena o recupera la información. " +
		"Está terminantemente prohibido usar frases como \"en el contexto del grafo de conocimiento\", \"en el grafo\", \"según los fragmentos proporcionados\", \"en la base de datos\", \"los nodos\", \"nodos semilla\" o \"en el contexto provisto\". " +
		"Dirígete al usuario con un lenguaje natural, directo y profesional.\n" +
		"3. Respaldo documental natural: Puedes citar con naturalidad los títulos de las secciones, políticas o normativas provistas cuando sea pertinente (por ejemplo, \"De acuerdo con [Título]...\"), sin referirte a ellos como nodos del sistema."

	userContent := fmt.Sprintf("Pregunta:\n%s\n\nDocumentación de referencia:\n%s", cleanedQuestion, contextPrompt)

	requestBody := chatCompletionRequest{
		Messages: []chatMessage{
			{
				Role:    "system",
				Content: systemInstruction,
			},
			{
				Role:    "user",
				Content: userContent,
			},
		},
		Temperature: 0.1,
		MaxTokens:   800,
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("error serializando solicitud al LLM: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/v1/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("error creando solicitud al LLM: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error conectando con el LLM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("el LLM respondió con HTTP %d", resp.StatusCode)
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return "", fmt.Errorf("error leyendo respuesta del LLM: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("el LLM respondió sin resultados")
	}

	return strings.TrimSpace(completion.Choices[0].Message.Content), nil
}
