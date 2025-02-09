package database

import (
	"context"
	"mealplanner/internal/database/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
    pool *pgxpool.Pool
    *db.Queries
}

func New(ctx context.Context, connString string) (*DB, error) {
    pool, err := pgxpool.New(ctx, connString)
    if err != nil {
        return nil, err
    }

    queries := db.New(pool)

    return &DB{
        pool:    pool,
        Queries: queries,
    }, nil
}

func (d *DB) Close() {
    d.pool.Close()
}

// WithTx begins a transaction and returns queries that use it
func (d *DB) WithTx(ctx context.Context, fn func(*db.Queries) error) error {
    tx, err := d.pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    qtx := d.Queries.WithTx(tx)
    if err := fn(qtx); err != nil {
        return err
    }

    return tx.Commit(ctx)
}
