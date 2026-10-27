package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"thinker/internal/config"
	"thinker/internal/database"

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

	var username string
	err = conn.QueryRow(context.Background(), "select username from users where id=$1", 2).Scan(&username)
	if err != nil {
		fmt.Printf("Error while retrieving rows: %w", err)
	}
	fmt.Println(username)

	//load templates and parse dirs
	router := gin.Default()
	router.HTMLRender = config.LoadTemplates("./templates/")

	router.GET("/ideas", func(c *gin.Context) {
		c.HTML(http.StatusOK, "ideas_list.html", gin.H{
			"title": "Mis ideas",
		})
	})

	router.Run(":8080")

}
