package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/models"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("Hello from thinker")

	//get base config
	baseConfig, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Error while loading env variables")
	}

	// base context
	ctx := context.Background()

	dbConnector := database.NewConnector(baseConfig)
	conn, err := dbConnector.Connect(ctx)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer conn.Close(ctx)

	//init queries
	queries := database.New(conn)

	//load templates and parse dirs
	router := gin.Default()
	router.HTMLRender = config.LoadTemplates("./templates/")

	router.GET("/ideas", func(c *gin.Context) {

		rows, err := conn.Query(c.Request.Context(), `
            SELECT title, content
            FROM ideas 
            ORDER BY created_at DESC;
        `)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error al consultar ideas: %v", err)
			return
		}
		defer rows.Close()

		var ideas []models.Idea
		for rows.Next() {
			var idea models.Idea
			err := rows.Scan(
				&idea.Title,
				&idea.Content,
			)
			if err != nil {
				c.String(http.StatusInternalServerError, "Error al procesar registros: %v", err)
				return
			}
			ideas = append(ideas, idea)
		}

		if err := rows.Err(); err != nil {
			c.String(http.StatusInternalServerError, "Error en la iteración: %v", err)
			return
		}

		c.HTML(http.StatusOK, "ideas_list.html", gin.H{
			"title": "Mis ideas",
			"ideas": ideas,
		})
	})

	router.GET("/users", func(c *gin.Context) {
		users, err := queries.ListUsers(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "No se pudieron obtener los usuarios"})
			return
		}

		c.JSON(http.StatusOK, users)
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "hello world from thinker",
		})
	})

	router.Run(":8080")

}
