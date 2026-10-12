package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func Open() (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// Run once, before starting API and workers (later a Helm migration Job).
// The transaction lock makes concurrent migration attempts safe.
func Migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(340034)"); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS orders (
 id BIGSERIAL PRIMARY KEY,
 item TEXT NOT NULL CHECK (length(item) BETWEEN 1 AND 200),
 quantity INTEGER NOT NULL CHECK (quantity BETWEEN 1 AND 1000),
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processed')),
 traceparent TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 processed_at TIMESTAMPTZ
 )`)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS orders_pending_idx ON orders(id) WHERE status = 'pending'`); err != nil {
		return err
	}
	return tx.Commit()
}
