package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/vntrieu/avalon/internal/database"
	"github.com/vntrieu/avalon/internal/httpapi"
)

func main() {
	_ = godotenv.Load()

	addr := getenv("AVALON_HTTP_ADDR", ":8080")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	migrationsDir := getenv("MIGRATIONS_DIR", "migrations")

	// Connect to PostgreSQL.
	ctx := context.Background()
	dbPool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer dbPool.Close()
	log.Println("connected to database")

	// Run pending migrations.
	if err := database.Migrate(ctx, dbPool, migrationsDir); err != nil {
		log.Fatalf("database migrate: %v", err)
	}
	log.Println("migrations up to date")

	tokenSecret := []byte(os.Getenv("WEBSOCKET_TOKEN_SECRET"))
	if len(tokenSecret) == 0 {
		tokenSecret = []byte("dev-secret-change-in-production")
	}

	// Pass nil for rateLimiter to disable; use httpapi.DefaultRateLimiter() to enable (20/min per IP).
	router := httpapi.NewRouter(dbPool, tokenSecret, nil)

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("avalon backend listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
