package config

import (
	"os"
	"strconv"
)

type Config struct {
	GRPCPort    int
	HealthPort  int
	BindAddress string

	JWTSigningKey       string
	AccessTokenTTLSecs  int
	RefreshTokenTTLSecs int

	// RS256 key paths (if set, RS256 is used instead of HS256)
	JWTRSAPrivateKeyPath string
	JWTRSAPublicKeyPath  string

	// Production mode: if true, requires explicit JWT_SECRET or RSA key paths
	ProductionMode bool

	DBHost    string
	DBPort    int
	DBUser    string
	DBPass    string
	DBName    string
	DBSSLMode string

	BcryptCost          int
	MaxFailedAttempts   int
	LockoutDurationMins int

	// Redis
	RedisURL string
}

func DefaultConfig() Config {
	return Config{
		GRPCPort:            50055,
		HealthPort:          8085,
		BindAddress:         "0.0.0.0",
		AccessTokenTTLSecs:  900,
		RefreshTokenTTLSecs: 86400,
		DBHost:              "localhost",
		DBPort:              5432,
		DBUser:              "auth",
		DBName:              "ace_auth",
		DBSSLMode:           "require",
		BcryptCost:          12,
		MaxFailedAttempts:   5,
		LockoutDurationMins: 30,
	}
}

func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("GRPC_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GRPCPort = port
		}
	}
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HealthPort = port
		}
	}
	if v := os.Getenv("BIND_ADDRESS"); v != "" {
		cfg.BindAddress = v
	}
	if v := os.Getenv("AUTH_JWT_SIGNING_KEY"); v != "" {
		cfg.JWTSigningKey = v
	}
	if v := os.Getenv("ACCESS_TOKEN_TTL_SECS"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil {
			cfg.AccessTokenTTLSecs = ttl
		}
	}
	if v := os.Getenv("REFRESH_TOKEN_TTL_SECS"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil {
			cfg.RefreshTokenTTLSecs = ttl
		}
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.DBHost = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.DBPort = port
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.DBUser = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.DBPass = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.DBName = v
	}
	if v := os.Getenv("DB_SSL_MODE"); v != "" {
		cfg.DBSSLMode = v
	}
	if v := os.Getenv("BCRYPT_COST"); v != "" {
		if cost, err := strconv.Atoi(v); err == nil {
			cfg.BcryptCost = cost
		}
	}
	if v := os.Getenv("MAX_FAILED_ATTEMPTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxFailedAttempts = n
		}
	}
	if v := os.Getenv("LOCKOUT_DURATION_MINS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LockoutDurationMins = n
		}
	}
	if v := os.Getenv("REDIS_URL"); v != "" {
		cfg.RedisURL = v
	}
	if v := os.Getenv("JWT_RSA_PRIVATE_KEY_PATH"); v != "" {
		cfg.JWTRSAPrivateKeyPath = v
	}
	if v := os.Getenv("JWT_RSA_PUBLIC_KEY_PATH"); v != "" {
		cfg.JWTRSAPublicKeyPath = v
	}
	if v := os.Getenv("PRODUCTION_MODE"); v != "" {
		cfg.ProductionMode = v == "true" || v == "1" || v == "yes"
	}

	return cfg
}
