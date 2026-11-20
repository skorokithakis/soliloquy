// Command qaseed fills a database with demo threads, comments, accounts and
// votes for manual QA. It is not part of the server and is not shipped in the
// Docker image.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("qaseed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dbPath := flag.String("db", "", "path to the SQLite database to seed")
	flag.Parse()

	if strings.TrimSpace(*dbPath) == "" {
		return errors.New("-db is required")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	ctx := context.Background()
	if err := database.Migrate(ctx); err != nil {
		return err
	}

	seeded, err := seed(ctx, database, time.Now())
	if err != nil {
		return err
	}
	if !seeded {
		slog.Info("QA data already present; nothing seeded", "db", *dbPath)
		return nil
	}

	slog.Info("seeded QA data", "db", *dbPath)
	return nil
}
