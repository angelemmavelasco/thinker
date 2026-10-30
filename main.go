package main

import (
	"context"
	"fmt"
	"log"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/routes"
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
	// initi router
	router := routes.SetUpRouter(queries)

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Error al ejecutar el servidor HTTP: %v", err)
	}

}
