//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal"
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
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/llm"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/middleware"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/infrastructure/storage"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/pkg/id"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/testhelpers/vectorpg"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type env struct {
	server *httptest.Server
	pool   *pgxpool.Pool
	store  storage.Client
}

var testEnv *env

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := vectorpg.Run(ctx)
	if err != nil {
		fmt.Printf("start postgres container: %v\n", err)
		os.Exit(1)
	}

	pgURL, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Printf("postgres connection string: %v\n", err)
		os.Exit(1)
	}

	pool, err := database.Connect(ctx, pgURL)
	if err != nil {
		fmt.Printf("connect: %v\n", err)
		os.Exit(1)
	}

	if err := database.RunMigrations(ctx, pool, "../../cmd/server/migrations"); err != nil {
		fmt.Printf("migrations: %v\n", err)
		os.Exit(1)
	}

	storeDir, err := os.MkdirTemp("", "jhic-e2e-store-")
	if err != nil {
		fmt.Printf("create store dir: %v\n", err)
		os.Exit(1)
	}
	store, err := storage.NewLocalClient(storage.LocalConfig{Dir: storeDir})
	if err != nil {
		fmt.Printf("new local client: %v\n", err)
		os.Exit(1)
	}

	usersRepo := authpg.NewUsersRepository(pool)
	sessionsRepo := authpg.NewSessionsRepository(pool)
	authSvc := auth.NewService(usersRepo, sessionsRepo)
	authHnd := auth.NewHandler(authSvc)

	userRepo := userpg.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	userHnd := user.NewHandler(userSvc)

	beritaRepo := beritapg.NewRepository(pool)
	beritaSvc := berita.NewService(beritaRepo)
	beritaHnd := berita.NewHandler(beritaSvc, store)

	pklRepo := pklpg.NewRepository(pool)
	pklSvc := pkl.NewService(pklRepo, userRepo)
	pklHnd := pkl.NewHandler(pklSvc)

	const matchStubOutput = `{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":60,"persentase_akuntansi":30,"persentase_hotel":10}`

	llmStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			var req struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			user := ""
			for _, m := range req.Messages {
				if m.Content != "" {
					user = m.Content
				}
			}
			content := "hai dari nexxa"
			switch {
			case strings.Contains(user, "<!-- CV START -->"):
				content = cvReviewStubOutput
			case strings.Contains(user, "jawaban_"):
				content = matchStubOutput
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			})
		case "/v1/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": make([]float32, 384)}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	llmClient := llm.NewClient(llm.Config{
		BaseURL:    llmStub.URL + "/v1",
		Model:      "test-model",
		Timeout:    5 * time.Second,
		EmbedModel: "test-embed",
	})
	chatSvc := chat.NewService(llmClient).
		WithRetrieval(chatpg.NewRepository(pool), llmClient, 5)
	chatHnd := chat.NewHandler(chatSvc, middleware.RateLimit(1000))
	matchSvc := match.NewService(llmClient)
	matchHnd := match.NewHandler(matchSvc, middleware.RateLimit(1000))

	tokenValidator := middleware.TokenValidator(auth.NewTokenValidator(sessionsRepo))
	authMw := middleware.Auth(tokenValidator)
	cvSvc := cvreview.NewService(llmClient)
	cvHnd := cvreview.NewHandler(cvSvc, authMw, middleware.RateLimit(1000))
	roleChecker := func(ctx context.Context, userID id.ID) (string, error) {
		u, err := userSvc.ByID(ctx, userID)
		if err != nil {
			return "", err
		}
		if u == nil {
			return "", fmt.Errorf("user not found")
		}
		return u.Role, nil
	}
	roleMw := middleware.RequireRole("jurnal")(roleChecker)

	router := internal.NewRouter(authHnd, userHnd, beritaHnd, pklHnd, chatHnd, matchHnd, cvHnd, authMw, roleMw, roleChecker, []string{"*"})
	server := httptest.NewServer(router)

	testEnv = &env{server: server, pool: pool, store: store}

	code := m.Run()
	server.Close()
	llmStub.Close()
	pool.Close()
	_ = pgContainer.Terminate(ctx)
	os.RemoveAll(storeDir)
	os.Exit(code)
}

func startE2E(t *testing.T) *env {
	t.Helper()
	_, err := testEnv.pool.Exec(context.Background(), `TRUNCATE pkl_approval_steps, pkl_requests, sessions, berita, users, kb_chunks, kb_documents CASCADE`)
	require.NoError(t, err)
	return testEnv
}

func promoteToAdmin(t *testing.T, e *env, userID id.ID) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `UPDATE users SET role = 'admin' WHERE id = $1`, userID)
	require.NoError(t, err)
}
