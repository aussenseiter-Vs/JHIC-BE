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
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/auth"
	authpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/auth/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita"
	beritapg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/berita/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/pkl"
	pklpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/pkl/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/user"
	userpg "github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/user/pg"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/database"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/middleware"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/n8n"
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
	beritaSvc := berita.NewService(beritaRepo)
	beritaHnd := berita.NewHandler(beritaSvc, b2Client)

	pklRepo := pklpg.NewRepository(pool)
	pklSvc := pkl.NewService(pklRepo, userRepo)
	pklHnd := pkl.NewHandler(pklSvc)

	n8nClient := n8n.NewClient(n8n.Config{
		BaseURL:      cfg.N8NBaseURL,
		ChatPath:     cfg.N8NChatPath,
		ChatUsername: cfg.N8NChatUsername,
		ChatPassword: cfg.N8NChatPassword,
		NexxaPath:    cfg.N8NNexxaPath,
		NexxaSecret:  cfg.N8NNexxaSecret,
		Timeout:      cfg.N8NTimeout,
	})
	chatSvc := chat.NewService(n8nClient)
	chatHnd := chat.NewHandler(chatSvc, middleware.RateLimit(cfg.AIRateLimit))
	matchSvc := match.NewService(n8nClient)
	matchHnd := match.NewHandler(matchSvc, middleware.RateLimit(cfg.AIRateLimit))

	tokenValidator := middleware.TokenValidator(auth.NewTokenValidator(sessionsRepo))
	authMw := middleware.Auth(tokenValidator)
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

	router := internal.NewRouter(authHnd, userHnd, beritaHnd, pklHnd, chatHnd, matchHnd, authMw, roleMw, roleChecker)

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