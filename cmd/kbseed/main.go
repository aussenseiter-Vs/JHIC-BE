package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/config"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/database"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/knowledge"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/llm"
)

func main() {
	cfg := config.Load()

	var dir string
	var batch int
	flag.StringVar(&dir, "dir", cfg.KBBaseDir, "directory of markdown knowledge files")
	flag.IntVar(&batch, "batch", knowledge.DefaultBatchSize, "embedding batch size")
	flag.Parse()

	if cfg.LLMBaseURL == "" && cfg.LLMEmbedURL == "" {
		log.Fatal("LLM_BASE_URL is required (see .env.example)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()
	if err := database.RunMigrations(ctx, pool, "cmd/server/migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	client := llm.NewClient(llm.Config{
		BaseURL:    cfg.LLMBaseURL,
		APIKey:     cfg.LLMAPIKey,
		Model:      cfg.LLMModel,
		Timeout:    cfg.LLMTimeout,
		MaxTokens:  cfg.LLMMaxTokens,
		EmbedURL:   cfg.LLMEmbedURL,
		EmbedModel: cfg.LLMEmbedModel,
	})

	if err := knowledge.Seed(ctx, pool, knowledge.Config{
		Dir:       dir,
		Embedder:  client,
		BatchSize: batch,
	}); err != nil {
		log.Fatalf("kb seed: %v", err)
	}
	log.Printf("kb seeded from %s", dir)
}