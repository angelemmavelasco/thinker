package handlers

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/services"
)

type IdeaHandler struct {
	service *services.IdeaService
}

func NewIdeaHandler(queries *database.Queries, llm *services.LLMService, embedding *services.EmbeddingService) *IdeaHandler {
	svc := services.NewIdeaService(queries, llm, embedding)
	go svc.ProcessPendingIdeas(context.Background())
	return &IdeaHandler{
		service: svc,
	}
}

func (h *IdeaHandler) ListIdeas(c *gin.Context) {
	ideas, err := h.service.ListIdeas(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al consultar el repositorio de ideas: %v", err)
		return
	}

	data := gin.H{
		"title": "Repositorio de ideas",
		"ideas": ideas,
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_ideas_list.html", data)
		return
	}

	config.Render(c, http.StatusOK, "ideas_list.html", data)
}

func (h *IdeaHandler) NewIdea(c *gin.Context) {
	data := gin.H{
		"title": "Nueva idea",
	}

	if c.GetHeader("HX-Request") == "true" {
		config.Render(c, http.StatusOK, "_ideas_new.html", data)
		return
	}

	config.Render(c, http.StatusOK, "ideas_new.html", data)
}

func (h *IdeaHandler) CreateIdea(c *gin.Context) {
	title := strings.TrimSpace(c.PostForm("title"))
	content := strings.TrimSpace(c.PostForm("content"))

	fileHeader, fileErr := c.FormFile("doc")
	if fileErr == nil && fileHeader != nil && fileHeader.Size > 0 {
		extractedText, err := services.ExtractTextFromUpload(fileHeader)
		if err != nil {
			data := gin.H{
				"title":     "Nueva idea",
				"error":     err.Error(),
				"ideaTitle": title,
				"content":   content,
			}
			if c.GetHeader("HX-Request") == "true" {
				config.Render(c, http.StatusBadRequest, "_ideas_new.html", data)
				return
			}
			config.Render(c, http.StatusBadRequest, "ideas_new.html", data)
			return
		}

		if title == "" {
			ext := filepath.Ext(fileHeader.Filename)
			title = strings.TrimSpace(strings.TrimSuffix(fileHeader.Filename, ext))
		}

		if content == "" {
			content = extractedText
		} else if extractedText != "" {
			content = content + "\n\n" + extractedText
		}
	}

	if title == "" || content == "" {
		data := gin.H{
			"title":     "Nueva idea",
			"error":     "El título y el contenido son obligatorios (o sube un archivo .txt, .md, .pdf, .docx, .csv).",
			"ideaTitle": title,
			"content":   content,
		}

		if c.GetHeader("HX-Request") == "true" {
			config.Render(c, http.StatusBadRequest, "_ideas_new.html", data)
			return
		}

		config.Render(c, http.StatusBadRequest, "ideas_new.html", data)
		return
	}

	_, err := h.service.CreateIdeasFromInput(c.Request.Context(), c.GetString("user"), title, content)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error al guardar la idea: %v", err)
		return
	}

	if c.GetHeader("HX-Request") == "true" {
		ideas, err := h.service.ListIdeas(c.Request.Context())
		if err != nil {
			c.String(http.StatusInternalServerError, "La idea se guardó, pero no se pudo actualizar el listado: %v", err)
			return
		}

		config.Render(c, http.StatusOK, "_ideas_list.html", gin.H{
			"title": "Repositorio de ideas",
			"ideas": ideas,
		})
		return
	}

	c.Redirect(http.StatusSeeOther, "/web/ideas/list")
}

func (h *IdeaHandler) NetworkView(c *gin.Context) {
	config.Render(c, http.StatusOK, "network.html", gin.H{
		"title": "Red de pensamiento",
	})
}

func (h *IdeaHandler) NetworkGraphJSON(c *gin.Context) {
	graph, err := h.service.GetNetworkGraph(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudo cargar la red de conocimiento"})
		return
	}

	c.JSON(http.StatusOK, graph)
}
