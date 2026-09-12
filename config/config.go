package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              int
	CORSAllowedOrigin []string
	DatabaseURL       string
	B2Endpoint        string
	B2KeyID           string
	B2AppKey          string
	B2Bucket          string
	B2Region          string

	LLMBaseURL    string
	LLMAPIKey     string
	LLMModel      string
	LLMTimeout    time.Duration
	LLMMaxTokens  int
	LLMEmbedURL   string
	LLMEmbedModel string

	KBBaseDir    string
	KBMaxResults int
	KBAutoSeed   bool

	ChatHistoryMax int
	ChatHistoryTTL time.Duration

	AIRateLimit int
}

func Load() *Config {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	if port == 0 {
		port = 8080
	}
	origin := strings.FieldsFunc(getEnv("CORS_ALLOWED_ORIGINS", "*"), func(r rune) bool { return r == ',' })
	for i, o := range origin {
		origin[i] = strings.TrimSpace(o)
	}
	rateLimit, _ := strconv.Atoi(getEnv("AI_RATE_LIMIT", "10"))
	if rateLimit == 0 {
		rateLimit = 10
	}
	llmTimeout, _ := strconv.Atoi(getEnv("LLM_TIMEOUT", "115"))
	llmMaxTokens, _ := strconv.Atoi(getEnv("LLM_MAX_TOKENS", "1500"))
	kbMaxResults, _ := strconv.Atoi(getEnv("KB_MAX_RESULTS", "5"))
	chatHistoryMax, _ := strconv.Atoi(getEnv("CHAT_HISTORY_MAX", "20"))
	chatHistoryTTL, _ := strconv.Atoi(getEnv("CHAT_HISTORY_TTL", "1800"))

	return &Config{
		Port:              port,
		CORSAllowedOrigin: origin,
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/jhic?sslmode=disable"),
		B2Endpoint:        getEnv("B2_ENDPOINT", "s3.eu-central-003.backblazeb2.com"),
		B2KeyID:           getEnv("B2_KEY_ID", ""),
		B2AppKey:          getEnv("B2_APP_KEY", ""),
		B2Bucket:          getEnv("B2_BUCKET", "jhic-berita-images"),
		B2Region:          resolveB2Region(getEnv("B2_REGION", ""), getEnv("B2_ENDPOINT", "s3.eu-central-003.backblazeb2.com")),

		LLMBaseURL:    getEnv("LLM_BASE_URL", ""),
		LLMAPIKey:     getEnv("LLM_API_KEY", ""),
		LLMModel:      getEnv("LLM_MODEL", "gpt-4o-mini"),
		LLMTimeout:    time.Duration(llmTimeout) * time.Second,
		LLMMaxTokens:  llmMaxTokens,
		LLMEmbedURL:   getEnv("LLM_EMBED_BASE_URL", "http://localhost:8081/v1"),
		LLMEmbedModel: getEnv("LLM_EMBED_MODEL", "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"),

		KBBaseDir:    getEnv("KB_DIR", "kb"),
		KBMaxResults: kbMaxResults,
		KBAutoSeed:   getEnv("KB_AUTO_SEED", "true") == "true",

		ChatHistoryMax: chatHistoryMax,
		ChatHistoryTTL: time.Duration(chatHistoryTTL) * time.Second,

		AIRateLimit: rateLimit,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func resolveB2Region(region, endpoint string) string {
	if region != "" {
		return region
	}
	host := endpoint
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+len("://"):]
	}
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if strings.HasPrefix(host, "s3.") && strings.HasSuffix(host, ".backblazeb2.com") {
		parts := strings.Split(host, ".")
		if len(parts) >= 4 && parts[1] != "" {
			return parts[1]
		}
	}
	return "us-east-005"
}