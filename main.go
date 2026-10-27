package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"thinker/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func main() {
	fmt.Println("Hello from thinker")

	//get base config
	baseConfig, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Error while loading env variables")
	}

	conn, err := pgx.Connect(context.Background(), baseConfig.ConnString)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer conn.Close(context.Background())

	var username string
	err = conn.QueryRow(context.Background(), "select username from users where id=$1", 1).Scan(&username)
	if err != nil {
		fmt.Errorf("Error while retrieving rows: %w", err)
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
