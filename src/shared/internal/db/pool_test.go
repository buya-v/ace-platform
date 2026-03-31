package db

import (
	"context"
	"os"
	"testing"
	"testing/fstest"
)

// --- Config tests ---

func TestDefaultConfig(t *testing.T) {
	// Unset all env vars to ensure defaults are used.
	envVars := []string{
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD",
		"DB_NAME", "DB_SSL_MODE", "DB_MAX_CONNS", "DB_MIN_CONNS",
	}
	originals := make(map[string]string)
	for _, k := range envVars {
		originals[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range originals {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}()

	cfg := DefaultConfig()

	if cfg.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Errorf("expected Port=5432, got %d", cfg.Port)
	}
	if cfg.User != "garudax" {
		t.Errorf("expected User=garudax, got %s", cfg.User)
	}
	if cfg.Password != "garudax_dev_password" {
		t.Errorf("expected Password=garudax_dev_password, got %s", cfg.Password)
	}
	if cfg.DBName != "garudax" {
		t.Errorf("expected DBName=garudax, got %s", cfg.DBName)
	}
	if cfg.SSLMode != "disable" {
		t.Errorf("expected SSLMode=disable, got %s", cfg.SSLMode)
	}
	if cfg.MaxConns != 10 {
		t.Errorf("expected MaxConns=10, got %d", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Errorf("expected MinConns=2, got %d", cfg.MinConns)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("DB_HOST", "db.prod.internal")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "s3cret")
	t.Setenv("DB_NAME", "exchange")
	t.Setenv("DB_SSL_MODE", "require")
	t.Setenv("DB_MAX_CONNS", "50")
	t.Setenv("DB_MIN_CONNS", "5")

	cfg := DefaultConfig()

	if cfg.Host != "db.prod.internal" {
		t.Errorf("expected Host=db.prod.internal, got %s", cfg.Host)
	}
	if cfg.Port != 5433 {
		t.Errorf("expected Port=5433, got %d", cfg.Port)
	}
	if cfg.User != "appuser" {
		t.Errorf("expected User=appuser, got %s", cfg.User)
	}
	if cfg.Password != "s3cret" {
		t.Errorf("expected Password=s3cret, got %s", cfg.Password)
	}
	if cfg.DBName != "exchange" {
		t.Errorf("expected DBName=exchange, got %s", cfg.DBName)
	}
	if cfg.SSLMode != "require" {
		t.Errorf("expected SSLMode=require, got %s", cfg.SSLMode)
	}
	if cfg.MaxConns != 50 {
		t.Errorf("expected MaxConns=50, got %d", cfg.MaxConns)
	}
	if cfg.MinConns != 5 {
		t.Errorf("expected MinConns=5, got %d", cfg.MinConns)
	}
}

func TestConfigInvalidEnvPort(t *testing.T) {
	t.Setenv("DB_PORT", "not-a-number")

	cfg := DefaultConfig()
	if cfg.Port != 5432 {
		t.Errorf("expected fallback Port=5432 for invalid env, got %d", cfg.Port)
	}
}

func TestConfigInvalidEnvMaxConns(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "abc")

	cfg := DefaultConfig()
	if cfg.MaxConns != 10 {
		t.Errorf("expected fallback MaxConns=10 for invalid env, got %d", cfg.MaxConns)
	}
}

func TestConfigDSN(t *testing.T) {
	cfg := Config{
		Host:     "myhost",
		Port:     5433,
		User:     "myuser",
		Password: "mypass",
		DBName:   "mydb",
		SSLMode:  "require",
		MaxConns: 10,
		MinConns: 2,
	}

	expected := "postgres://myuser:mypass@myhost:5433/mydb?sslmode=require"
	got := cfg.DSN()
	if got != expected {
		t.Errorf("expected DSN=%s, got %s", expected, got)
	}
}

// --- Validation tests ---

func TestValidateConfig_Valid(t *testing.T) {
	cfg := Config{
		Host:     "localhost",
		Port:     5432,
		User:     "garudax",
		Password: "pass",
		DBName:   "garudax",
		SSLMode:  "disable",
		MaxConns: 10,
		MinConns: 2,
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidateConfig_EmptyHost(t *testing.T) {
	cfg := Config{Host: "", Port: 5432, User: "u", DBName: "d", MaxConns: 1}
	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error for empty host")
	}
}

func TestValidateConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too_high", 70000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Host: "h", Port: tt.port, User: "u", DBName: "d", MaxConns: 1}
			if err := validateConfig(cfg); err == nil {
				t.Errorf("expected error for port=%d", tt.port)
			}
		})
	}
}

func TestValidateConfig_EmptyUser(t *testing.T) {
	cfg := Config{Host: "h", Port: 5432, User: "", DBName: "d", MaxConns: 1}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for empty user")
	}
}

func TestValidateConfig_EmptyDBName(t *testing.T) {
	cfg := Config{Host: "h", Port: 5432, User: "u", DBName: "", MaxConns: 1}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for empty db name")
	}
}

func TestValidateConfig_MaxConnsZero(t *testing.T) {
	cfg := Config{Host: "h", Port: 5432, User: "u", DBName: "d", MaxConns: 0}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for max_conns=0")
	}
}

func TestValidateConfig_MinConnsNegative(t *testing.T) {
	cfg := Config{Host: "h", Port: 5432, User: "u", DBName: "d", MaxConns: 5, MinConns: -1}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for negative min_conns")
	}
}

func TestValidateConfig_MinExceedsMax(t *testing.T) {
	cfg := Config{Host: "h", Port: 5432, User: "u", DBName: "d", MaxConns: 5, MinConns: 10}
	if err := validateConfig(cfg); err == nil {
		t.Error("expected error for min_conns > max_conns")
	}
}

// --- envOrDefault tests ---

func TestEnvOrDefault_Fallback(t *testing.T) {
	t.Setenv("TEST_DB_ENV_UNUSED_KEY_XYZ", "")
	os.Unsetenv("TEST_DB_ENV_UNUSED_KEY_XYZ")
	got := envOrDefault("TEST_DB_ENV_UNUSED_KEY_XYZ", "fallback")
	if got != "fallback" {
		t.Errorf("expected fallback, got %s", got)
	}
}

func TestEnvOrDefault_Set(t *testing.T) {
	t.Setenv("TEST_DB_ENV_SET_KEY", "value")
	got := envOrDefault("TEST_DB_ENV_SET_KEY", "fallback")
	if got != "value" {
		t.Errorf("expected value, got %s", got)
	}
}

func TestEnvOrDefaultInt_Fallback(t *testing.T) {
	os.Unsetenv("TEST_DB_INT_UNUSED_KEY")
	got := envOrDefaultInt("TEST_DB_INT_UNUSED_KEY", 42)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestEnvOrDefaultInt_Set(t *testing.T) {
	t.Setenv("TEST_DB_INT_SET_KEY", "99")
	got := envOrDefaultInt("TEST_DB_INT_SET_KEY", 42)
	if got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
}

func TestEnvOrDefaultInt_Invalid(t *testing.T) {
	t.Setenv("TEST_DB_INT_BAD_KEY", "not-int")
	got := envOrDefaultInt("TEST_DB_INT_BAD_KEY", 42)
	if got != 42 {
		t.Errorf("expected fallback 42 for invalid int, got %d", got)
	}
}

// --- Migration file collection tests ---

func TestCollectMigrationFiles_SortedOrder(t *testing.T) {
	testFS := fstest.MapFS{
		"V003__add_index.sql":    {Data: []byte("CREATE INDEX ...")},
		"V001__create_table.sql": {Data: []byte("CREATE TABLE ...")},
		"V002__add_column.sql":   {Data: []byte("ALTER TABLE ...")},
		"readme.txt":             {Data: []byte("not a migration")},
	}

	files, err := CollectMigrationFiles(testFS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	expected := []string{
		"V001__create_table.sql",
		"V002__add_column.sql",
		"V003__add_index.sql",
	}
	for i, f := range files {
		if f != expected[i] {
			t.Errorf("files[%d] = %s, expected %s", i, f, expected[i])
		}
	}
}

func TestCollectMigrationFiles_Empty(t *testing.T) {
	testFS := fstest.MapFS{}
	files, err := CollectMigrationFiles(testFS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestCollectMigrationFiles_NestedDirectories(t *testing.T) {
	testFS := fstest.MapFS{
		"sub/V001__nested.sql": {Data: []byte("SELECT 1")},
		"V002__top.sql":        {Data: []byte("SELECT 2")},
	}

	files, err := CollectMigrationFiles(testFS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	// Lexicographic: "V002__top.sql" < "sub/V001__nested.sql"
	if files[0] != "V002__top.sql" {
		t.Errorf("expected V002__top.sql first, got %s", files[0])
	}
}

// --- MigrationResult tests ---

func TestMigrationResult_Fields(t *testing.T) {
	r := MigrationResult{
		Filename: "V001__init.sql",
		Applied:  true,
		Error:    nil,
	}
	if r.Filename != "V001__init.sql" {
		t.Errorf("unexpected filename: %s", r.Filename)
	}
	if !r.Applied {
		t.Error("expected Applied=true")
	}
	if r.Error != nil {
		t.Errorf("expected nil error, got %v", r.Error)
	}
}

// --- HealthStatus tests ---

func TestHealthStatus_Fields(t *testing.T) {
	s := HealthStatus{
		Healthy: true,
		Error:   "",
		PoolStat: PoolStats{
			TotalConns: 5,
			MaxConns:   10,
		},
	}
	if !s.Healthy {
		t.Error("expected Healthy=true")
	}
	if s.PoolStat.TotalConns != 5 {
		t.Errorf("expected TotalConns=5, got %d", s.PoolStat.TotalConns)
	}
	if s.PoolStat.MaxConns != 10 {
		t.Errorf("expected MaxConns=10, got %d", s.PoolStat.MaxConns)
	}
}

// --- Pool Config accessor test ---

func TestPoolConfigAccessor(t *testing.T) {
	// We can't create a real Pool without a DB, but we can test
	// that Config() returns what was stored.
	cfg := Config{
		Host:     "testhost",
		Port:     5432,
		User:     "testuser",
		Password: "testpass",
		DBName:   "testdb",
		SSLMode:  "disable",
		MaxConns: 20,
		MinConns: 4,
	}
	p := &Pool{config: cfg}
	got := p.Config()
	if got.Host != "testhost" {
		t.Errorf("expected Host=testhost, got %s", got.Host)
	}
	if got.MaxConns != 20 {
		t.Errorf("expected MaxConns=20, got %d", got.MaxConns)
	}
}

// --- NewPool validation test (no DB required) ---

func TestNewPool_InvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"empty_host", Config{Host: "", Port: 5432, User: "u", DBName: "d", MaxConns: 1}},
		{"bad_port", Config{Host: "h", Port: 0, User: "u", DBName: "d", MaxConns: 1}},
		{"empty_user", Config{Host: "h", Port: 5432, User: "", DBName: "d", MaxConns: 1}},
		{"empty_db", Config{Host: "h", Port: 5432, User: "u", DBName: "", MaxConns: 1}},
		{"zero_max", Config{Host: "h", Port: 5432, User: "u", DBName: "d", MaxConns: 0}},
		{"min_gt_max", Config{Host: "h", Port: 5432, User: "u", DBName: "d", MaxConns: 2, MinConns: 5}},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPool(ctx, tt.cfg)
			if err == nil {
				t.Error("expected error for invalid config")
			}
		})
	}
}
