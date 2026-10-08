package postgres

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stevenchen/llm-gateway/internal/config"
	"github.com/stevenchen/llm-gateway/internal/pkg/pgerr"
	"github.com/stevenchen/llm-gateway/migrations"
)

// Runs against a real PostgreSQL when LLM_GATEWAY_TEST_PG_HOST is set (e.g. the
// docker-compose dev database); otherwise it is skipped.
//
//	LLM_GATEWAY_TEST_PG_HOST=127.0.0.1 LLM_GATEWAY_TEST_PG_PORT=5433 \
//	LLM_GATEWAY_TEST_PG_USER=llmgateway LLM_GATEWAY_TEST_PG_PASSWORD=llmgateway go test ./internal/adapter/repository/postgres
func TestMigrateEmptyDatabaseEndToEnd(t *testing.T) {
	host := os.Getenv("LLM_GATEWAY_TEST_PG_HOST")
	if host == "" {
		t.Skip("LLM_GATEWAY_TEST_PG_HOST not set")
	}
	port, _ := strconv.Atoi(os.Getenv("LLM_GATEWAY_TEST_PG_PORT"))
	if port == 0 {
		port = 5432
	}
	admin := config.PostgresConfig{Host: host, Port: port, User: os.Getenv("LLM_GATEWAY_TEST_PG_USER"),
		Password: os.Getenv("LLM_GATEWAY_TEST_PG_PASSWORD"), DBName: "postgres", SSLMode: "disable",
		MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute}

	adminDB, err := Connect(admin)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("llmgw_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { adminDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)") })

	cfg := admin
	cfg.DBName = name
	db, err := Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 1. an untouched database is reported as not migrated — the original bug
	st := CheckSchema(ctx, db)
	if st.OK || st.Problem == "" {
		t.Fatalf("empty database must not look ready: %+v", st)
	}
	err = db.Exec("SELECT 1 FROM api_keys").Error
	if !pgerr.IsUndefinedRelation(err) {
		t.Fatalf("expected undefined-relation error before migrating, got %v", err)
	}

	// 2. Migrate brings it to the version this binary requires
	latest, _ := migrations.LatestVersion()
	v, err := Migrate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if v != latest {
		t.Fatalf("migrated to %d, want %d", v, latest)
	}
	if st := CheckSchema(ctx, db); !st.OK {
		t.Fatalf("schema should be ready after migrate: %+v", st)
	}
	if err := db.Exec("SELECT 1 FROM api_keys, metrics_minute, applications, budgets").Error; err != nil {
		t.Fatalf("expected tables missing after migrate: %v", err)
	}

	// 3. idempotent: running again on an up-to-date database is a no-op
	if v2, err := Migrate(cfg); err != nil || v2 != latest {
		t.Fatalf("second migrate: v=%d err=%v", v2, err)
	}
}
