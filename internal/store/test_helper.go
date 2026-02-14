package store

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vntrieu/avalon/internal/database"
)

// SetupTestDB creates a test database connection pool.
// It expects DATABASE_URL environment variable to be set.
// This function is exported so it can be used by other test packages.
func SetupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("DATABASE_URL or TEST_DATABASE_URL environment variable is required for tests")
	}

	ctx := context.Background()
	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	// Clean up test data before running tests
	if err := cleanupTestData(ctx, pool); err != nil {
		t.Logf("warning: failed to cleanup test data: %v", err)
	}

	return pool
}

// cleanupTestData removes all test data from the database.
func cleanupTestData(ctx context.Context, pool *pgxpool.Pool) error {
	// Delete in reverse order of foreign key dependencies
	tables := []string{
		"chat_messages",
		"game_events",
		"game_state_snapshots",
		"game_players",
		"games",
		"room_players",
		"rooms",
	}

	for _, table := range tables {
		if _, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}

	return nil
}
