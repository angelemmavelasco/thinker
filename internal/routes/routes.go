package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/handlers"
	"thinker/internal/urls"
)

func SetUpRouter(queries *database.Queries) *gin.Engine {
	// base router
	router := gin.Default()
	router.Use(config.GlobalContextMiddleware())
	router.Static("/static", "./static")
	router.HTMLRender = config.LoadTemplates("./templates/")

	// needed handlers
	ideaHandler := handlers.NewIdeaHandler(queries)
	userHandler := handlers.NewUserHandler(queries)

	router.GET(urls.Get("api:users:list"), userHandler.ListUsers)
	router.GET(urls.Get("ideas:list"), ideaHandler.ListIdeas)
	router.GET(urls.Get("ideas:new"), ideaHandler.NewIdea)

	router.GET(urls.Get("users:list"), userHandler.ListUsersView)
	router.GET(urls.Get("landing"), func(c *gin.Context) {
		c.HTML(http.StatusOK, "landing.html", gin.H{})
	})

	return router
}
