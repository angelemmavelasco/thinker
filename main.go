package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"thinker/internal/config"
	"thinker/internal/database"
	"thinker/internal/routes"
	"thinker/internal/services"
)

func main() {
	fmt.Println("Hello from thinker")

	baseConfig, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Error while loading env variables")
	}

	ctx := context.Background()

	dbConnector := database.NewConnector(baseConfig)
	conn, err := dbConnector.Connect(ctx)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer func(conn *sql.DB) {
		err := conn.Close()
		if err != nil {
			log.Printf("Error while closing database connection: %v", err)
		}
	}(conn)

	if err := database.InitSchema(ctx, conn); err != nil {
		log.Fatal(err.Error())
	}
	if baseConfig.Debug {
		if err := database.EnsureDevelopmentUser(ctx, conn); err != nil {
			log.Fatal(err.Error())
		}
	}

	queries := database.New(conn)
	llmService := services.NewLLMService(baseConfig.LLMBaseURL, baseConfig.LLMApiKey)
	embeddingService := services.NewEmbeddingService(baseConfig.EmbeddingBaseURL, baseConfig.LLMApiKey)
	router := routes.SetUpRouter(queries, llmService, embeddingService, baseConfig)

	if err := router.Run("127.0.0.1:8080"); err != nil {
		log.Fatalf("Error al ejecutar el servidor HTTP: %v", err)
	}
}
