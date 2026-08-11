package dbtest

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewPool starts a throwaway Postgres container, applies all migrations
// from backend/db/migrations, and returns a ready-to-use pool. The
// container and pool are torn down automatically via t.Cleanup.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kanvas_test"),
		tcpostgres.WithUsername("kanvas"),
		tcpostgres.WithPassword("kanvas"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	m, err := migrate.New("file://"+filepath.ToSlash(migrationsDir()), dsn)
	if err != nil {
		t.Fatalf("creating migrator: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("running migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// migrationsDir resolves backend/db/migrations relative to this source
// file, so it works no matter which package's test calls NewPool.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile: .../backend/internal/platform/db/dbtest/dbtest.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "db", "migrations")
}
