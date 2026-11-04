// Package db owns the connection pool and the tenant-scoped transaction (ADR-0011 §2).
// Every tenant query goes through WithTenant, so a missing tenant filter is impossible
// to write by accident: the database itself filters the rows (RLS).
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool and checks that the database answers.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// WithTenant runs fn in one transaction whose tenant is tenantID. The setting is
// transaction-local (set_config(..., true)), so it cannot leak to the next request
// that reuses the pooled connection. fn's error rolls the transaction back.
func WithTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(tx pgx.Tx) error) error {
	if tenantID == "" {
		return errors.New("db: WithTenant needs a tenant id")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tenant transaction: %w", err)
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed, which is expected.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("set tenant: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tenant transaction: %w", err)
	}
	return nil
}
