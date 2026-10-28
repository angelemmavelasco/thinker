package routes

import (
	"net/http"
	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/handlers"
	"thinker/internal/urls"

	"github.com/gin-gonic/gin"
)

func SetUpRouter(queries *database.Queries) *gin.Engine {
	// base router
	router := gin.Default()
	router.Static("/static", "./static")
	router.HTMLRender = config.LoadTemplates("./templates/")

	// needed handlers
	ideaHandler := handlers.NewIdeaHandler(queries)
	userHandler := handlers.NewUserHandler(queries)

	router.GET(urls.Get("api:users:list"), userHandler.ListUsers)
	router.GET(urls.Get("ideas:list"), ideaHandler.ListIdeas)
	router.GET(urls.Get("users:list"), userHandler.ListUsersView)
	router.GET(urls.Get("landing"), func(c *gin.Context) {
		c.HTML(http.StatusOK, "landing.html", gin.H{})
	})

	return router
}
