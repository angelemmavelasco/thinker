package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type BaseConfig struct {
	Debug      bool
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
	// first, we validate that we are not in debug mode
	debugVar := os.Getenv("DEBUG")
	var debug bool
	switch debugVar {
	case "true", "1", "True", "yes", "Yes", "Y", "y", "T", "t":
		debug = true
	case "false", "False", "F", "f", "No", "no", "0", "N", "n":
		debug = false
	default:
		debug = false
	}

	// model selection
	llmApiKey := os.Getenv("LLM_API_KEY")

	// db config
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	connString := os.Getenv("CONN_STRING")

	config := BaseConfig{
		Debug: debug,
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
