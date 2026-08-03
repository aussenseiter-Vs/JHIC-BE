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
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	testBucket = "test-bucket"
	minioUser  = "minioadmin"
	minioPass  = "minioadmin"
)

type env struct {
	server   *httptest.Server
	pool     *pgxpool.Pool
	store    storage.Client
	verifyS3 *s3.Client
}

var testEnv *env

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("jhic"),
		postgres.WithUsername("jhic"),
		postgres.WithPassword("jhic"),
		postgres.BasicWaitStrategies(),
	)
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

	minioContainer, err := minio.Run(ctx, "minio/minio:latest",
		minio.WithUsername(minioUser),
		minio.WithPassword(minioPass),
		testcontainers.WithTmpfs(map[string]string{"/data": "rw"}),
	)
	if err != nil {
		fmt.Printf("start minio container: %v\n", err)
		os.Exit(1)
	}

	endpoint, err := minioContainer.ConnectionString(ctx)
	if err != nil {
		fmt.Printf("minio connection string: %v\n", err)
		os.Exit(1)
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		})),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(minioUser, minioPass, "")),
		config.WithRegion("us-east-1"),
	)
	if err != nil {
		fmt.Printf("aws config: %v\n", err)
		os.Exit(1)
	}
	verifyS3 := s3.NewFromConfig(awsCfg)

	if _, err := verifyS3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(testBucket)}); err != nil {
		fmt.Printf("create bucket: %v\n", err)
		os.Exit(1)
	}

	store, err := storage.NewB2Client(ctx, storage.B2Config{
		Endpoint: endpoint,
		Region:   "us-east-1",
		KeyID:    minioUser,
		AppKey:   minioPass,
		Bucket:   testBucket,
	})
	if err != nil {
		fmt.Printf("new b2 client: %v\n", err)
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

	n8nStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/chat":
			json.NewEncoder(w).Encode(map[string]string{"output": "hai dari nexxa"})
		case "/nexxa":
			w.Write([]byte(`{"nama_jurusan":"PPLG","alasan":"cocok","persentase_pplg":60,"persentase_akuntansi":30,"persentase_hotel":10}`))
		default:
			http.NotFound(w, r)
		}
	}))
	n8nClient := n8n.NewClient(n8n.Config{
		BaseURL:   n8nStub.URL,
		ChatPath:  "/chat",
		NexxaPath: "/nexxa",
		Timeout:   5 * time.Second,
	})
	chatSvc := chat.NewService(n8nClient)
	chatHnd := chat.NewHandler(chatSvc, middleware.RateLimit(1000))
	matchSvc := match.NewService(n8nClient)
	matchHnd := match.NewHandler(matchSvc, middleware.RateLimit(1000))

	tokenValidator := middleware.TokenValidator(auth.NewTokenValidator(sessionsRepo))
	authMw := middleware.Auth(tokenValidator)
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

	router := internal.NewRouter(authHnd, userHnd, beritaHnd, pklHnd, chatHnd, matchHnd, authMw, roleMw, roleChecker)
	server := httptest.NewServer(router)

	testEnv = &env{server: server, pool: pool, store: store, verifyS3: verifyS3}

	code := m.Run()
	server.Close()
	n8nStub.Close()
	pool.Close()
	_ = pgContainer.Terminate(ctx)
	_ = minioContainer.Terminate(ctx)
	os.Exit(code)
}

func startE2E(t *testing.T) *env {
	t.Helper()
	_, err := testEnv.pool.Exec(context.Background(), `TRUNCATE pkl_approval_steps, pkl_requests, sessions, berita, users CASCADE`)
	require.NoError(t, err)
	return testEnv
}

func promoteToAdmin(t *testing.T, e *env, userID id.ID) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `UPDATE users SET role = 'admin' WHERE id = $1`, userID)
	require.NoError(t, err)
}