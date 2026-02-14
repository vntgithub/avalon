package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate runs database migrations using goose.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	// Convert pgxpool.Pool to *sql.DB for goose compatibility
	// Extract the connection config and create a stdlib DB connection
	connConfig := pool.Config().ConnConfig
	db := stdlib.OpenDB(*connConfig)
	defer db.Close()

	// Set goose dialect to postgres
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	// Run migrations
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	return nil
}
