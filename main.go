package main

import (
	"context"
	"database/sql"
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
	defer func(conn *sql.DB) {
		err := conn.Close()
		if err != nil {
			log.Printf("Error while closing database connection: %v", err)
		}
	}(conn)

	//define base schema
	if err := database.InitSchema(ctx, conn); err != nil {
		log.Fatal(err.Error())
	}
	if baseConfig.Debug {
		if err := database.EnsureDevelopmentUser(ctx, conn); err != nil {
			log.Fatal(err.Error())
		}
	}

	//init queries
	queries := database.New(conn)
	// initi router
	router := routes.SetUpRouter(queries)

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Error al ejecutar el servidor HTTP: %v", err)
	}

}
