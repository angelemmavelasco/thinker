package main

import (
	"fmt"
	"log"
	"thinker/internal/config"
)

func main() {
	fmt.Println("Hello from thinker")

	baseConfig, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Error while loading env variables")
	}
	fmt.Printf("Base config loaded successfully: %+v\n", *baseConfig)

}
