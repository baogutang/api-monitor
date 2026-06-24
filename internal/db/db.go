package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockID int64 = 821905230457001

type migrationQuerier interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)
	}()

	if err := ensureMigrationTable(ctx, conn); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	for _, file := range files {
		applied, err := migrationApplied(ctx, conn, file)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", file); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func ensureMigrationTable(ctx context.Context, q migrationQuerier) error {
	_, err := q.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`)
	return err
}

func migrationApplied(ctx context.Context, q migrationQuerier, version string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", version).Scan(&exists)
	return exists, err
}
