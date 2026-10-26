package main

import (
	"fmt"
	"log"
	"net/http"
	"thinker/internal/config"

	"github.com/gin-gonic/gin"
)

func main() {
	fmt.Println("Hello from thinker")

	//get base config
	_, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Error while loading env variables")
	}
	fmt.Println("Base config loaded successfully\n")

	//load templates and parse dirs
	router := gin.Default()
	router.HTMLRender = config.LoadTemplates("./templates/")

	//test================
	router.GET("/ideas", func(c *gin.Context) {
		c.HTML(http.StatusOK, "ideas_list.html", gin.H{
			"title": "Mis ideas",
		})
	})

	router.Run(":8080")

}
