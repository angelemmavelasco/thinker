package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type BaseConfig struct {
	Debug                     bool
	AppBaseURL                string
	JWTSecret                 []byte
	SecureCookies             bool
	ExternalLLMProvider       bool
	ExternalEmbeddingProvider bool
	LLMApiKey                 string
	LLMBaseURL                string
	LLMModel                  string
	EmbeddingApiKey           string
	EmbeddingBaseURL          string
	EmbeddingModel            string
	DBHost                    string
	DBPort                    string
	DBName                    string
	DBUsername                string
	DBPassword                string
	ConnString                string
	GoogleClientID            string
	GoogleClientSecret        string
	GoogleRedirectURL         string
	SMTPHost                  string
	SMTPPort                  string
	SMTPUsername              string
	SMTPPassword              string
	SMTPFrom                  string
}

func parseBoolEnv(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	switch strings.ToLower(v) {
	case "true", "1", "yes", "y", "t":
		return true
	default:
		return false
	}
}

func LoadConfig() (*BaseConfig, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("[config] No se encontró archivo .env local, usando variables de entorno del sistema")
	}

	debug := parseBoolEnv("DEBUG")

	appBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	if appBaseURL == "" {
		appBaseURL = "http://127.0.0.1:8080"
	}

	jwtSecret, err := resolveJWTSecret(debug)
	if err != nil {
		return nil, err
	}

	secureCookies := !debug || strings.HasPrefix(strings.ToLower(appBaseURL), "https://")

	externalLLMProvider := parseBoolEnv("EXTERNAL_LLM_PROVIDER")
	externalEmbeddingProvider := parseBoolEnv("EXTERNAL_EMBEDDING_PROVIDER")

	var llmApiKey, llmBaseURL, llmModel string
	if externalLLMProvider {
		llmApiKey = strings.TrimSpace(os.Getenv("LLM_API_KEY"))
		llmBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/")
		if llmBaseURL == "" {
			llmBaseURL = "https://api.minimax.io/v1"
		}
		llmModel = strings.TrimSpace(os.Getenv("LLM_MODEL"))
		if llmModel == "" {
			llmModel = "MiniMax-Text-01"
		}
	} else {
		llmBaseURL = "http://127.0.0.1:8081/v1"
		llmModel = "qwen2.5-3b-instruct"
		llmApiKey = ""
	}

	var embeddingApiKey, embeddingBaseURL, embeddingModel string
	if externalEmbeddingProvider {
		embeddingApiKey = strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY"))
		if embeddingApiKey == "" {
			embeddingApiKey = llmApiKey
		}
		embeddingBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL")), "/")
		if embeddingBaseURL == "" {
			embeddingBaseURL = "https://api.minimax.io/v1"
		}
		embeddingModel = strings.TrimSpace(os.Getenv("EMBEDDING_MODEL"))
		if embeddingModel == "" {
			embeddingModel = "embo-01"
		}
	} else {
		embeddingBaseURL = "http://127.0.0.1:8082/v1"
		embeddingModel = "bge-small-en-v1.5"
		embeddingApiKey = ""
	}

	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbName := os.Getenv("DB_NAME")
	dbUsername := os.Getenv("DB_USERNAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	connString := os.Getenv("CONN_STRING")

	googleClientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	googleClientSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	googleRedirectURL := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	if googleRedirectURL == "" {
		googleRedirectURL = appBaseURL + "/auth/google/callback"
	}

	smtpHost := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	smtpPort := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if smtpPort == "" {
		smtpPort = "587"
	}
	smtpUsername := strings.TrimSpace(os.Getenv("SMTP_USER"))
	smtpPassword := os.Getenv("SMTP_PASS")
	smtpFrom := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if smtpFrom == "" {
		smtpFrom = "no-reply@thinker.local"
	}

	cfg := BaseConfig{
		Debug:                     debug,
		AppBaseURL:                appBaseURL,
		JWTSecret:                 jwtSecret,
		SecureCookies:             secureCookies,
		ExternalLLMProvider:       externalLLMProvider,
		ExternalEmbeddingProvider: externalEmbeddingProvider,
		LLMApiKey:                 llmApiKey,
		LLMBaseURL:                llmBaseURL,
		LLMModel:                  llmModel,
		EmbeddingApiKey:           embeddingApiKey,
		EmbeddingBaseURL:          embeddingBaseURL,
		EmbeddingModel:            embeddingModel,
		DBHost:                    dbHost,
		DBPort:                    dbPort,
		DBName:                    dbName,
		DBUsername:                dbUsername,
		DBPassword:                dbPassword,
		ConnString:                connString,
		GoogleClientID:            googleClientID,
		GoogleClientSecret:        googleClientSecret,
		GoogleRedirectURL:         googleRedirectURL,
		SMTPHost:                  smtpHost,
		SMTPPort:                  smtpPort,
		SMTPUsername:              smtpUsername,
		SMTPPassword:              smtpPassword,
		SMTPFrom:                  smtpFrom,
	}
	fmt.Println("Base config loaded successfully:")
	return &cfg, nil
}

func resolveJWTSecret(debug bool) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if raw != "" {
		if decodedHex, err := hex.DecodeString(raw); err == nil && len(decodedHex) >= 32 {
			return decodedHex, nil
		}
		if decodedB64, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decodedB64) >= 32 {
			return decodedB64, nil
		}
		if len(raw) >= 32 {
			return []byte(raw), nil
		}
		if !debug {
			return nil, errors.New("JWT_SECRET tiene menos de 32 bytes (256 bits) de entropía")
		}
	}

	if !debug {
		return nil, errors.New("JWT_SECRET es obligatorio en producción y debe tener al menos 256 bits (32 bytes)")
	}

	ephemeral := make([]byte, 32)
	if _, err := rand.Read(ephemeral); err != nil {
		return nil, fmt.Errorf("error generando clave efímera para desarrollo: %w", err)
	}
	log.Println("[SECURITY WARNING] JWT_SECRET no definido o menor a 256 bits. Se generó una clave aleatoria efímera de 256 bits sólo para esta instancia de desarrollo.")
	return ephemeral, nil
}
