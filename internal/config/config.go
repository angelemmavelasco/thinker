package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type BaseConfig struct {
	LLMApiKey  string
	DBHost     string
	DBUsername string
	DBPassword string
	DBName     string
}

func LoadConfig() (*BaseConfig, error) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error while loading env file")
	}

	llmApiKey := os.Getenv("LLM_API_KEY")
	dbHost := os.Getenv("DBHOST")
	dbUsername := os.Getenv("DBUSERNAME")
	dbPassword := os.Getenv("DBPASSWORD")
	dbName := os.Getenv("DBNAME")

	config := BaseConfig{
		LLMApiKey:  llmApiKey,
		DBHost:     dbHost,
		DBUsername: dbUsername,
		DBPassword: dbPassword,
		DBName:     dbName,
	}

	fmt.Println("Base config loaded successfully:")
	return &config, nil
}
