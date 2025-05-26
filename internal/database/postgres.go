package database

import (
	"context"
	"log"
	"mealplanner/internal/database/db"

	"github.com/jackc/pgx/v5/tracelog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
	*db.Queries
}
type qLogger struct{}

func (l qLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	log.Default().Printf("Message from db: %v\n", msg)
}

func New(ctx context.Context, connString string) (*DB, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	// logger := qLogger{}
	// Add logging configuration
	// config.connconfig.tracer = &tracelog.tracelog{
	// 	loglevel: tracelog.logleveltrace,
	// 	logger:   logger,
	// }

	pool, err := pgxpool.NewWithConfig(ctx, config)
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
