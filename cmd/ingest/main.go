package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/your-org/ai-sa-assistant/internal/config"
	"go.uber.org/zap"
)

func main() {
	docsPath := flag.String("docs-path", "./docs", "Path to documents directory")
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Starting ingestion service",
		zap.String("docs_path", *docsPath),
		zap.String("chroma_url", cfg.Chroma.URL))

	// TODO: Implement ingestion pipeline
	// 1. Read documents from docs path
	// 2. Parse and chunk documents
	// 3. Generate embeddings
	// 4. Store in ChromaDB

	fmt.Println("Ingestion service - pipeline not yet implemented")
	os.Exit(0)
}
