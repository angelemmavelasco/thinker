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
	DBPort     string
	DBName     string
	DBUsername string
	DBPassword string
	ConnString string
}

func LoadConfig() (*BaseConfig, error) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error while loading env file")
	}

	llmApiKey := os.Getenv("LLM_API_KEY")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	connString := os.Getenv("CONN_STRING")

	config := BaseConfig{
		LLMApiKey:  llmApiKey,
		DBHost:     dbHost,
		DBPort:     dbPort,
		DBName:     dbName,
		DBUsername: dbUsername,
		DBPassword: dbPassword,
		ConnString: connString,
	}
	fmt.Println("Base config loaded successfully:")
	return &config, nil
}
