package main

import (
	"context"
	"log"
	"os"

	"cloud.google.com/go/bigquery"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

func main() {
	// 1. Load the `.env` file from project root
	if err := godotenv.Load(); err != nil {
		log.Println("No `.env` file found; falling back to system environment variables")
	}

	// 2. Read the variables
	projectID := os.Getenv("GCP_PROJECT_ID")
	serviceAccountPath := os.Getenv("GCP_SERVICE_ACCOUNT_PATH")
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")

	if projectID == "" || serviceAccountPath == "" || geminiAPIKey == "" {
		log.Fatal("GCP_PROJECT_ID or GCP_SERVICE_ACCOUNT_PATH or GEMINI_API_KEY is not set")
	}

	ctx := context.Background()

	// 3. Initialize BigQuery with the credentials from .env
	bqClient, err := bigquery.NewClient(
		ctx,
		projectID,
		option.WithAuthCredentialsFile(option.ServiceAccount, serviceAccountPath),
	)
	if err != nil {
		log.Fatalf("Failed to create BigQuery client: %v", err)
	}
	defer bqClient.Close()

	log.Println("BigQuery client initialized successfully")
}
