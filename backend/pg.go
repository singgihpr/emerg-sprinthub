package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
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

// ---- per-request transaction + RLS context ----

type pgTxKey struct{}

func pgTxFromContext(ctx context.Context) (pgx.Tx, error) {
	tx, ok := ctx.Value(pgTxKey{}).(pgx.Tx)
	if !ok || tx == nil {
		return nil, errors.New("no postgres transaction in context")
	}
	return tx, nil
}

// pgTxMiddleware opens a Postgres transaction for each request, sets the
// RLS context variables, and stores the tx in the request context. All
// repository helpers read the tx from context so RLS policies stay in effect.
func pgTxMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if pgPool == nil {
			return next(c)
		}
		ctx := c.Request().Context()
		tx, err := pgPool.Begin(ctx)
		if err != nil {
			return echo.NewHTTPError(503, "database unavailable")
		}
		defer func() {
			if r := recover(); r != nil {
				_ = tx.Rollback(ctx)
				panic(r)
			}
		}()

		ctx = context.WithValue(ctx, pgTxKey{}, tx)
		c.SetRequest(c.Request().WithContext(ctx))

		if userID := extractUserIDFromRequest(c.Request()); userID != "" {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.current_user', $1, true)", userID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if orgID := extractOrgIDFromPath(c.Request().URL.Path); orgID != "" {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.current_org', $1, true)", orgID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}

		err = next(c)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
}

func extractUserIDFromRequest(r *http.Request) string {
	token := ""
	if ck, err := r.Cookie("access_token"); err == nil {
		token = ck.Value
	}
	if token == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = auth[7:]
		}
	}
	if token == "" {
		return ""
	}
	tok, err := jwt.Parse(token, func(*jwt.Token) (interface{}, error) { return jwtSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !tok.Valid {
		return ""
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func extractOrgIDFromPath(path string) string {
	parts := strings.Split(path, "/")
	// /api/orgs/:org_id/...  => ["", "api", "orgs", ":org_id", ...]
	if len(parts) >= 4 && parts[2] == "orgs" {
		return parts[3]
	}
	return ""
}

// ---- query helpers (map-shaped, minimal SQL builder) ----

func pgFindOne(ctx context.Context, table string, filter map[string]any, proj []string) (map[string]any, error) {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	cols := "*"
	if len(proj) > 0 {
		cols = strings.Join(proj, ", ")
	}
	where, args := buildWhere(filter)
	q := fmt.Sprintf("SELECT %s FROM %s%s LIMIT 1", cols, table, where)
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanNextRow(rows)
}

func pgFindMany(ctx context.Context, table string, filter map[string]any, proj []string, order string, limit int64) ([]map[string]any, error) {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return nil, err
	}
	cols := "*"
	if len(proj) > 0 {
		cols = strings.Join(proj, ", ")
	}
	where, args := buildWhere(filter)
	q := fmt.Sprintf("SELECT %s FROM %s%s", cols, table, where)
	if order != "" {
		q += " ORDER BY " + order
	}
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowsToMaps(rows)
}

func pgInsert(ctx context.Context, table string, doc map[string]any) error {
	if len(doc) == 0 {
		return errors.New("empty document")
	}
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return err
	}
	cols, placeholders, args := buildInsert(doc)
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, cols, placeholders)
	_, err = tx.Exec(ctx, q, args...)
	return err
}

func pgUpdate(ctx context.Context, table string, filter, set map[string]any) error {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return err
	}
	var parts []string
	var args []any
	i := 1
	for k, v := range set {
		parts = append(parts, fmt.Sprintf("%s = $%d", k, i))
		args = append(args, v)
		i++
	}
	var conds []string
	for k, v := range filter {
		if v == nil {
			conds = append(conds, fmt.Sprintf("%s IS NULL", k))
		} else {
			conds = append(conds, fmt.Sprintf("%s = $%d", k, i))
			args = append(args, v)
			i++
		}
	}
	q := fmt.Sprintf("UPDATE %s SET %s", table, strings.Join(parts, ", "))
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	_, err = tx.Exec(ctx, q, args...)
	return err
}

func pgDelete(ctx context.Context, table string, filter map[string]any) error {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return err
	}
	where, args := buildWhere(filter)
	q := fmt.Sprintf("DELETE FROM %s%s", table, where)
	_, err = tx.Exec(ctx, q, args...)
	return err
}

// pgExec runs an arbitrary query against the request tx.
func pgExec(ctx context.Context, sql string, args ...any) error {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, sql, args...)
	return err
}

// pgPoolExec runs outside a request tx (seed, cron, background jobs).
func pgPoolExec(ctx context.Context, sql string, args ...any) error {
	if pgPool == nil {
		return errors.New("postgres not connected")
	}
	_, err := pgPool.Exec(ctx, sql, args...)
	return err
}

// pgPoolFind is for background tasks that need to read across tenants.
func pgPoolFind(ctx context.Context, table string, filter map[string]any) ([]map[string]any, error) {
	if pgPool == nil {
		return nil, errors.New("postgres not connected")
	}
	cols := "*"
	where, args := buildWhere(filter)
	q := fmt.Sprintf("SELECT %s FROM %s%s", cols, table, where)
	rows, err := pgPool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRowsToMaps(rows)
}

func pgPoolInsert(ctx context.Context, table string, doc map[string]any) error {
	if pgPool == nil {
		return errors.New("postgres not connected")
	}
	cols, placeholders, args := buildInsert(doc)
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, cols, placeholders)
	_, err := pgPool.Exec(ctx, q, args...)
	return err
}

func pgPoolUpdate(ctx context.Context, table string, filter, set map[string]any) error {
	if pgPool == nil {
		return errors.New("postgres not connected")
	}
	var parts []string
	var args []any
	i := 1
	for k, v := range set {
		parts = append(parts, fmt.Sprintf("%s = $%d", k, i))
		args = append(args, v)
		i++
	}
	var conds []string
	for k, v := range filter {
		if v == nil {
			conds = append(conds, fmt.Sprintf("%s IS NULL", k))
		} else {
			conds = append(conds, fmt.Sprintf("%s = $%d", k, i))
			args = append(args, v)
			i++
		}
	}
	q := fmt.Sprintf("UPDATE %s SET %s", table, strings.Join(parts, ", "))
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	_, err := pgPool.Exec(ctx, q, args...)
	return err
}

func pgPoolFindOne(ctx context.Context, table string, filter map[string]any) (map[string]any, error) {
	if pgPool == nil {
		return nil, errors.New("postgres not connected")
	}
	where, args := buildWhere(filter)
	q := fmt.Sprintf("SELECT * FROM %s%s LIMIT 1", table, where)
	rows, err := pgPool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanNextRow(rows)
}

// ---- SQL builders (identifiers are hard-coded, values are parameterized) ----

func buildWhere(filter map[string]any) (string, []any) {
	if len(filter) == 0 {
		return "", nil
	}
	var conds []string
	var args []any
	i := 1
	for k, v := range filter {
		if v == nil {
			conds = append(conds, fmt.Sprintf("%s IS NULL", k))
		} else {
			conds = append(conds, fmt.Sprintf("%s = $%d", k, i))
			args = append(args, v)
			i++
		}
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func buildInsert(doc map[string]any) (cols, placeholders string, args []any) {
	var keys []string
	var ph []string
	i := 1
	for k, v := range doc {
		keys = append(keys, k)
		ph = append(ph, fmt.Sprintf("$%d", i))
		args = append(args, v)
		i++
	}
	return strings.Join(keys, ", "), strings.Join(ph, ", "), args
}

func buildSet(set map[string]any) (string, []any) {
	var parts []string
	var args []any
	i := 1
	for k, v := range set {
		parts = append(parts, fmt.Sprintf("%s = $%d", k, i))
		args = append(args, v)
		i++
	}
	return strings.Join(parts, ", "), args
}

// ---- scanning ----

func scanNextRow(rows pgx.Rows) (map[string]any, error) {
	desc := rows.FieldDescriptions()
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	m := make(map[string]any, len(desc))
	for i, fd := range desc {
		m[fd.Name] = vals[i]
	}
	return m, nil
}

func scanRowsToMaps(rows pgx.Rows) ([]map[string]any, error) {
	var out []map[string]any
	for rows.Next() {
		m, err := scanNextRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func pgCount(ctx context.Context, table string, filter map[string]any) (int64, error) {
	tx, err := pgTxFromContext(ctx)
	if err != nil {
		return 0, err
	}
	where, args := buildWhere(filter)
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", table, where)
	var n int64
	if err := tx.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
