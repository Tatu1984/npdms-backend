package config

import "os"

type Config struct {
	DatabaseURL     string
	RedisURL        string
	MinioEndpoint   string
	MinioAccessKey  string
	MinioSecretKey  string
	MinioBucket     string
	MinioPublicURL  string
	MinioUseSSL     bool
	JWTSecret       string
	Port            string
	Env             string
}

func Load() *Config {
	cfg := &Config{
		DatabaseURL:    getEnvRequired("DATABASE_URL"),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
		MinioEndpoint:  getEnvRequired("MINIO_ENDPOINT"),
		MinioAccessKey: getEnvRequired("MINIO_ACCESS_KEY"),
		MinioSecretKey: getEnvRequired("MINIO_SECRET_KEY"),
		MinioBucket:    getEnv("MINIO_BUCKET", "npdms"),
		MinioPublicURL: getEnv("MINIO_PUBLIC_URL", ""),
		MinioUseSSL:    getEnv("MINIO_USE_SSL", "false") == "true",
		JWTSecret:      getEnvRequired("JWT_SECRET"),
		Port:           getEnv("PORT", "8080"),
		Env:            getEnv("ENV", "development"),
	}
	if cfg.MinioPublicURL == "" {
		protocol := "http"
		if cfg.MinioUseSSL {
			protocol = "https"
		}
		cfg.MinioPublicURL = protocol + "://" + cfg.MinioEndpoint
	}
	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvRequired(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic("required environment variable " + key + " is not set")
	}
	return value
}
