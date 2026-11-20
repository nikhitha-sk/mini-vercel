// Package config loads service configuration from environment variables.
// Every service (api, worker, edge) reads the same struct and ignores the
// fields it does not need, so one .env file drives the whole platform.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string
	RedisAddr   string
	RedisPass   string

	// Object storage (SeaweedFS or MinIO locally, Cloudflare R2 in production).
	S3Endpoint  string
	S3Region    string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string

	// EncryptionKey is a 32-byte key (hex or base64) for AES-256-GCM env vars.
	EncryptionKey string

	// Public routing. Deployments are served at <id>.<RootDomain>[:PublicPort].
	RootDomain   string
	PublicScheme string
	PublicPort   string

	APIAddr      string
	EdgeAddr     string
	APIToken     string // optional bearer token; empty = open (local dev)
	DashboardDir string // optional: serve the static dashboard from the API
	CORSOrigins  []string

	// Build sandbox.
	BuildImage       string
	BuildCPUs        float64
	BuildMemoryMB    int64
	BuildPIDs        int64
	BuildTimeout     time.Duration
	WorkerConcurrent int
}

func Load() Config {
	loadDotEnv(".env")
	return Config{
		DatabaseURL: env("MV_DATABASE_URL", "postgres://minivercel:minivercel@localhost:5433/minivercel?sslmode=disable"),
		RedisAddr:   env("MV_REDIS_ADDR", "localhost:6380"),
		RedisPass:   env("MV_REDIS_PASSWORD", ""),

		S3Endpoint:  env("MV_S3_ENDPOINT", "http://localhost:9000"),
		S3Region:    env("MV_S3_REGION", "us-east-1"),
		S3AccessKey: env("MV_S3_ACCESS_KEY", "minivercel"),
		S3SecretKey: env("MV_S3_SECRET_KEY", "minivercel-secret"),
		S3Bucket:    env("MV_S3_BUCKET", "deployments"),

		EncryptionKey: env("MV_ENCRYPTION_KEY", "6d696e692d76657263656c2d6465762d6b65792d33322d62797465732d212121"),

		RootDomain:   env("MV_ROOT_DOMAIN", "localhost"),
		PublicScheme: env("MV_PUBLIC_SCHEME", "http"),
		PublicPort:   env("MV_PUBLIC_PORT", "8080"),

		APIAddr:      env("MV_API_ADDR", ":4000"),
		EdgeAddr:     env("MV_EDGE_ADDR", ":8080"),
		APIToken:     env("MV_API_TOKEN", ""),
		DashboardDir: env("MV_DASHBOARD_DIR", ""),
		CORSOrigins:  strings.Split(env("MV_CORS_ORIGINS", "http://localhost:3000"), ","),

		BuildImage:       env("MV_BUILD_IMAGE", "node:20-alpine"),
		BuildCPUs:        envFloat("MV_BUILD_CPUS", 1),
		BuildMemoryMB:    int64(envInt("MV_BUILD_MEMORY_MB", 1024)),
		BuildPIDs:        int64(envInt("MV_BUILD_PIDS", 256)),
		BuildTimeout:     time.Duration(envInt("MV_BUILD_TIMEOUT_SEC", 600)) * time.Second,
		WorkerConcurrent: envInt("MV_WORKER_CONCURRENCY", 2),
	}
}

// PublicURL returns the externally reachable URL for a hostname label,
// e.g. "dpl-abc123" -> "http://dpl-abc123.localhost:8080".
func (c Config) PublicURL(label string) string {
	host := label + "." + c.RootDomain
	if c.PublicPort != "" && !(c.PublicScheme == "https" && c.PublicPort == "443") && !(c.PublicScheme == "http" && c.PublicPort == "80") {
		host += ":" + c.PublicPort
	}
	return fmt.Sprintf("%s://%s", c.PublicScheme, host)
}

// Hostname returns the bare hostname for a label ("portfolio" -> "portfolio.localhost").
func (c Config) Hostname(label string) string { return label + "." + c.RootDomain }

func env(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(env(k, "")); err == nil {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(env(k, ""), 64); err == nil {
		return v
	}
	return def
}
