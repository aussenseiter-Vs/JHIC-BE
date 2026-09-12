package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/config"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/analytics"
	analyticspg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/analytics/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/auth"
	authpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/auth/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita"
	beritapg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	chatpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/cvreview"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/pkl"
	pklpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/pkl/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/user"
	userpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/user/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/database"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/knowledge"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/llm"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/middleware"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/storage"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/joho/godotenv"
)

func main() {
	exe, _ := os.Executable()
	_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	_ = godotenv.Load()
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if err := database.RunMigrations(ctx, pool, "cmd/server/migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	usersRepo := authpg.NewUsersRepository(pool)
	sessionsRepo := authpg.NewSessionsRepository(pool)
	authSvc := auth.NewService(usersRepo, sessionsRepo)
	authHnd := auth.NewHandler(authSvc)

	userRepo := userpg.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	userHnd := user.NewHandler(userSvc)

	if cfg.B2Endpoint == "" || cfg.B2KeyID == "" || cfg.B2AppKey == "" {
		log.Fatalf("b2 storage: B2_ENDPOINT, B2_KEY_ID and B2_APP_KEY must be set (see .env.example)")
	}

	b2Cfg := storage.B2Config{
		Endpoint: cfg.B2Endpoint,
		Region:   cfg.B2Region,
		KeyID:    cfg.B2KeyID,
		AppKey:   cfg.B2AppKey,
		Bucket:   cfg.B2Bucket,
	}
	b2Client, err := storage.NewB2Client(ctx, b2Cfg)
	if err != nil {
		log.Fatalf("b2 storage: %v", err)
	}

	beritaRepo := beritapg.NewRepository(pool)
	analyticsRepo := analyticspg.NewRepository(pool)
	analyticsSvc := analytics.NewService(analyticsRepo)
	analyticsHnd := analytics.NewHandler(analyticsRepo)
	beritaSvc := berita.NewService(beritaRepo)
	beritaHnd := berita.NewHandler(beritaSvc, b2Client, analyticsSvc)

	pklRepo := pklpg.NewRepository(pool)
	pklSvc := pkl.NewService(pklRepo, userRepo)
	pklHnd := pkl.NewHandler(pklSvc)

	if cfg.LLMBaseURL == "" {
		log.Fatalf("llm: LLM_BASE_URL must be set (see .env.example)")
	}

	llmClient := llm.NewClient(llm.Config{
		BaseURL:    cfg.LLMBaseURL,
		APIKey:     cfg.LLMAPIKey,
		Model:      cfg.LLMModel,
		Timeout:    cfg.LLMTimeout,
		MaxTokens:  cfg.LLMMaxTokens,
		EmbedURL:   cfg.LLMEmbedURL,
		EmbedModel: cfg.LLMEmbedModel,
	})
	kbRepo := chatpg.NewRepository(pool)
	chatSvc := chat.NewService(llmClient).
		WithRetrieval(kbRepo, llmClient, cfg.KBMaxResults).
		WithMemory(cfg.ChatHistoryMax, cfg.ChatHistoryTTL)
	chatHnd := chat.NewHandler(chatSvc, middleware.RateLimit(cfg.AIRateLimit), analyticsSvc)
	matchSvc := match.NewService(llmClient)
	matchHnd := match.NewHandler(matchSvc, middleware.RateLimit(cfg.AIRateLimit), analyticsSvc)

	if cfg.KBAutoSeed {
		go func() {
			seedCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			count, err := knowledge.Count(seedCtx, pool)
			if err != nil || count > 0 {
				return
			}
			if err := knowledge.Seed(seedCtx, pool, knowledge.Config{
				Dir:      cfg.KBBaseDir,
				Embedder: llmClient,
			}); err != nil {
				log.Printf("kb auto-seed: %v", err)
			}
		}()
	}

	tokenValidator := middleware.TokenValidator(auth.NewTokenValidator(sessionsRepo))
	authMw := middleware.Auth(tokenValidator)
	cvSvc := cvreview.NewService(llmClient)
	cvHnd := cvreview.NewHandler(cvSvc, authMw, middleware.RateLimit(cfg.AIRateLimit))
	roleCheck := userSvc.ByID
	roleChecker := func(ctx context.Context, userID id.ID) (string, error) {
		u, err := roleCheck(ctx, userID)
		if err != nil {
			return "", err
		}
		if u == nil {
			return "", fmt.Errorf("user not found")
		}
		return u.Role, nil
	}
	roleMw := middleware.RequireRole("jurnal")(roleChecker)

	router := internal.NewRouter(authHnd, userHnd, beritaHnd, pklHnd, chatHnd, matchHnd, cvHnd, authMw, roleMw, roleChecker, cfg.CORSAllowedOrigin, analyticsHnd)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("server listening on :%d", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}
