package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var pgPool *pgxpool.Pool

func connectPostgres(ctx context.Context, dsn string) error {
	if dsn == "" {
		return nil
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("pgx connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pgx ping: %w", err)
	}
	pgPool = pool
	if err := runMigrations(ctx, pool); err != nil {
		return fmt.Errorf("pg migrations: %w", err)
	}
	log.Println("postgres connected and migrated")
	return nil
}

type migration struct {
	version string
	up      string
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var ups []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) < 1 {
			continue
		}
		b, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return err
		}
		ups = append(ups, migration{version: parts[0], up: string(b)})
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].version < ups[j].version })

	for _, m := range ups {
		var applied bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", m.version,
		).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err := pool.Exec(ctx, m.up); err != nil {
			return fmt.Errorf("migration %s: %w", m.version, err)
		}
		if _, err := pool.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", m.version,
		); err != nil {
			return err
		}
		log.Printf("applied migration %s", m.version)
	}
	return nil
}

func closePostgres() {
	if pgPool != nil {
		pgPool.Close()
	}
}

// setRLSUser configures the Postgres RLS variable for this request.
// Callers must run it inside a transaction for the settings to affect
// subsequent queries on the same connection.
func setRLSUser(ctx context.Context, userID string) error {
	if pgPool == nil {
		return nil
	}
	_, err := pgPool.Exec(ctx, "SELECT set_config('app.current_user', $1, true)", userID)
	return err
}

func setRLSTenant(ctx context.Context, orgID string) error {
	if pgPool == nil {
		return nil
	}
	_, err := pgPool.Exec(ctx, "SELECT set_config('app.current_org', $1, true)", orgID)
	return err
}
