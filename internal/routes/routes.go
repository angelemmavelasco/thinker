package routes

import (
	"net/http"
	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/handlers"

	"github.com/gin-gonic/gin"
)

type Router interface {
	NewRouter()
}

func SetUpRouter(queries *database.Queries) *gin.Engine {
	// base router
	router := gin.Default()
	router.HTMLRender = config.LoadTemplates("./templates/")

	// needed handlers
	ideaHandler := handlers.NewIdeaHandler(queries)
	userHandler := handlers.NewUserHandler(queries)

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "hello world from thinker",
		})
	})

	api := router.Group("/api")
	{
		users := api.Group("/users")
		{
			users.GET("", userHandler.ListUsers)
		}
	}

	web := router.Group("/web")
	{
		ideas := web.Group("/ideas")
		{
			ideas.GET("", ideaHandler.ListIdeas)
		}
	}

	return router
}
