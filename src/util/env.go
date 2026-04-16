package util

import (
	"log"

	"github.com/joho/godotenv"
)

func LoadEnv() {
	if err := godotenv.Load(".env"); err == nil {
		return
	}

	log.Println("No .env file found, loading example.env")
	if err := godotenv.Load("example.env"); err != nil {
		log.Fatal("Error loading .env file")
	}
}
