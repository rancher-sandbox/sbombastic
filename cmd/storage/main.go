package main

import (
	"log/slog"
	"os"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rancher/sbombastic/cmd/storage/server"
	"github.com/rancher/sbombastic/internal/storage"
)

func main() {
	// TODO: add CLI flags
	opts := slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &opts)).With("component", "storage")
	ctx := genericapiserver.SetupSignalContext()

	dbURI, err := os.ReadFile("/pg/uri")
	if err != nil {
		logger.Error("failed to read database URI", "error", err)
		os.Exit(1)
	}

	db, err := pgxpool.New(ctx, string(dbURI))
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		os.Exit(1)
	}

	// Run migrations
	if _, err := db.Exec(ctx, storage.CreateImageTableSQL); err != nil {
		logger.Error("failed to create image table", "error", err)
		os.Exit(1)
	}
	if _, err := db.Exec(ctx, storage.CreateSBOMTableSQL); err != nil {
		logger.Error("failed to create sbom table", "error", err)
		os.Exit(1)
	}
	if _, err := db.Exec(ctx, storage.CreateVulnerabilityReportTableSQL); err != nil {
		logger.Error("failed to create vulnerability report table", "error", err)
		os.Exit(1)
	}

	options := server.NewWardleServerOptions(db, logger)
	cmd := server.NewCommandStartWardleServer(ctx, options)
	code := cli.Run(cmd)

	db.Close()
	os.Exit(code)
}
