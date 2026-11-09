package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/handlers"
	"thinker/internal/services"
	"thinker/internal/urls"
)

func SetUpRouter(queries *database.Queries, llm *services.LLMService, embedding *services.EmbeddingService) *gin.Engine {
	// base router
	router := gin.Default()
	router.Use(config.GlobalContextMiddleware())
	router.Static("/static", "./static")
	router.HTMLRender = config.LoadTemplates("./templates/")

	// needed handlers
	ideaHandler := handlers.NewIdeaHandler(queries, llm, embedding)
	userHandler := handlers.NewUserHandler(queries)

	router.GET(urls.Get("api:users:list"), userHandler.ListUsers)
	router.GET(urls.Get("api:network:graph"), ideaHandler.NetworkGraphJSON)
	router.GET(urls.Get("network:view"), ideaHandler.NetworkView)
	router.GET(urls.Get("ideas:list"), ideaHandler.ListIdeas)
	router.GET(urls.Get("ideas:new"), ideaHandler.NewIdea)
	router.POST(urls.Get("ideas:new"), ideaHandler.CreateIdea)
	router.GET(urls.Get("ideas:detail:route"), ideaHandler.IdeaDetail)
	router.GET(urls.Get("ideas:edit:route"), ideaHandler.EditIdeaView)
	router.POST(urls.Get("ideas:edit:route"), ideaHandler.UpdateIdea)
	router.POST(urls.Get("ideas:reanalyze:route"), ideaHandler.ReanalyzeIdea)
	router.POST(urls.Get("ideas:delete:route"), ideaHandler.DeleteIdeaFragment)
	router.POST(urls.Get("ideas:delete_group:route"), ideaHandler.DeleteIdeaGroup)

	router.GET(urls.Get("users:list"), userHandler.ListUsersView)
	router.GET(urls.Get("users:new"), userHandler.NewUser)
	router.POST(urls.Get("users:new"), userHandler.CreateUser)
	router.GET(urls.Get("landing"), func(c *gin.Context) {
		c.HTML(http.StatusOK, "landing.html", gin.H{})
	})

	return router
}
