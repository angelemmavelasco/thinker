package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("Hello from thinker")

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error while loading env file")
	}

	apiKey := os.Getenv("LLM_API_KEY")
	fmt.Println(apiKey)

}
