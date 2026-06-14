// Package config 复刻 apps/api/src/config.ts：env 优先级（repo 根 .env 先、本地 .env 不覆盖）+ 生产守卫。
package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type StorageBackend string

const (
	BackendMinio StorageBackend = "minio"
	BackendS3    StorageBackend = "s3"
)

type Storage struct {
	Backend        StorageBackend
	Endpoint       string
	AccessKey      string
	SecretKey      string
	Bucket         string
	Region         string
	ForcePathStyle bool
}

type OnlyOffice struct {
	DSURL                   string
	APIPublicURL            string
	JWTEnabled              bool
	JWTSecret               string
	PluginURL               string
	OpenTokenTTLSeconds     int
	DownloadTokenTTLSeconds int
	CallbackTokenTTLSeconds int
}

type Model struct {
	CredentialSecret      string
	HealthTTLSeconds      int
	HealthDownThreshold   int
	ParseWorkerIntervalMs int
}

type Config struct {
	NodeEnv       string
	Port          int
	Host          string
	SessionSecret string
	WebOrigin     string
	DatabaseURL   string
	Storage       Storage
	OnlyOffice    OnlyOffice
	Model         Model
}

func (c Config) IsProd() bool { return c.NodeEnv == "production" }

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// resolveSecret 复刻 config.ts 的 resolveOnlyofficeJwtSecret / resolveModelCredentialSecret：
// 显式配置则用；生产缺失则 panic；开发缺失则用本地占位密钥并告警。
func resolveSecret(envKey, devDefault, nodeEnv string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if nodeEnv == "production" {
		panic(envKey + " 必须在生产环境显式配置，禁止使用默认值")
	}
	log.Printf("[config] %s 未设置，开发环境使用本地占位密钥（仅限本机 POC，勿用于部署）", envKey)
	return devDefault
}

// Load 读取环境并构造 Config。等价 config.ts 顶层逻辑。
func Load() Config {
	// 与 config.ts 一致：先加载 repo 根 .env（生效优先），再加载本地 .env（不覆盖已存在变量）。
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load()

	nodeEnv := env("NODE_ENV", "development")

	backend := StorageBackend(env("STORAGE_BACKEND", string(BackendMinio)))
	var st Storage
	st.Backend = backend
	if backend == BackendS3 {
		st.Endpoint = os.Getenv("S3_ENDPOINT")
		st.AccessKey = os.Getenv("S3_ACCESS_KEY")
		st.SecretKey = os.Getenv("S3_SECRET_KEY")
		st.Bucket = env("S3_BUCKET", "medoffice")
		st.Region = env("S3_REGION", "us-east-1")
		st.ForcePathStyle = false
	} else {
		st.Endpoint = env("MINIO_ENDPOINT", "http://localhost:9000")
		st.AccessKey = env("MINIO_ACCESS_KEY", "minioadmin")
		st.SecretKey = env("MINIO_SECRET_KEY", "minioadmin")
		st.Bucket = env("MINIO_BUCKET", "medoffice")
		st.Region = env("MINIO_REGION", "us-east-1")
		st.ForcePathStyle = true
	}

	webOrigin := env("WEB_ORIGIN", "http://localhost:5173")

	return Config{
		NodeEnv:       nodeEnv,
		Port:          atoi(os.Getenv("API_PORT"), 3001),
		Host:          env("API_HOST", "0.0.0.0"),
		SessionSecret: resolveSecret("SESSION_SECRET", "dev-session-secret-change-me", nodeEnv),
		WebOrigin:     webOrigin,
		DatabaseURL:   env("DATABASE_URL", "postgres://medoffice:medoffice@localhost:5432/medoffice"),
		Storage:       st,
		OnlyOffice: OnlyOffice{
			DSURL:                   env("ONLYOFFICE_DS_URL", "http://localhost:8080"),
			APIPublicURL:            env("API_PUBLIC_URL", "http://host.docker.internal:3001"),
			JWTEnabled:              os.Getenv("ONLYOFFICE_JWT_ENABLED") != "false",
			JWTSecret:               resolveSecret("ONLYOFFICE_JWT_SECRET", "dev-local-onlyoffice-jwt-do-not-deploy", nodeEnv),
			PluginURL:               env("ONLYOFFICE_PLUGIN_URL", webOrigin+"/onlyoffice-plugin/"),
			OpenTokenTTLSeconds:     atoi(os.Getenv("EDITOR_OPEN_TOKEN_TTL"), 900),
			DownloadTokenTTLSeconds: atoi(os.Getenv("EDITOR_DOWNLOAD_TOKEN_TTL"), 300),
			CallbackTokenTTLSeconds: atoi(os.Getenv("EDITOR_CALLBACK_TTL"), 7200),
		},
		Model: Model{
			CredentialSecret:      resolveSecret("MODEL_CREDENTIAL_SECRET", "dev-local-model-credential-do-not-deploy", nodeEnv),
			HealthTTLSeconds:      atoi(os.Getenv("MODEL_HEALTH_TTL"), 60),
			HealthDownThreshold:   atoi(os.Getenv("MODEL_HEALTH_DOWN_THRESHOLD"), 3),
			ParseWorkerIntervalMs: atoi(os.Getenv("PARSE_WORKER_INTERVAL_MS"), 5000),
		},
	}
}
