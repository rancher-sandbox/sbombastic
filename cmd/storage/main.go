package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"os"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rancher/sbombastic/cmd/storage/server"
	"github.com/rancher/sbombastic/internal/storage"
)

func main() {
	os.Exit(run())
}

func run() int {
	// TODO: add CLI flags
	opts := slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &opts)).With("component", "storage")
	ctx := genericapiserver.SetupSignalContext()

	dbURI, err := os.ReadFile("/pg/uri")
	if err != nil {
		logger.Error("failed to read database URI", "error", err)
		return 1
	}

	config, err := pgxpool.ParseConfig(string(dbURI))
	if err != nil {
		logger.Error("failed to parse database URI", "error", err)
		return 1
	}

	// Use the BeforeConnect callback so that whenever a connection is created or reset,
	// the TLS configuration is reapplied.
	// This ensures that certificates are reloaded from disk if they have been updated.
	// See https://github.com/jackc/pgx/discussions/2103
	config.BeforeConnect = func(_ context.Context, connConfig *pgx.ConnConfig) error {
		connConfig.Fallbacks = nil // disable TLS fallback to force TLS connection

		var serverCA []byte
		serverCA, err = os.ReadFile("/pg/tls/server/ca.crt")
		if err != nil {
			return fmt.Errorf("failed to read database server CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(serverCA) {
			return errors.New("failed to append database server CA certificate to pool")
		}

		connConfig.TLSConfig = &tls.Config{
			RootCAs:            caCertPool,
			ServerName:         config.ConnConfig.Host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false,
		}

		return nil
	}

	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		return 1
	}
	defer db.Close()

	// Run migrations
	if _, err := db.Exec(ctx, storage.CreateImageTableSQL); err != nil {
		logger.Error("failed to create image table", "error", err)
		return 1
	}
	if _, err := db.Exec(ctx, storage.CreateSBOMTableSQL); err != nil {
		logger.Error("failed to create sbom table", "error", err)
		return 1
	}
	if _, err := db.Exec(ctx, storage.CreateVulnerabilityReportTableSQL); err != nil {
		logger.Error("failed to create vulnerability report table", "error", err)
		return 1
	}

	options := server.NewWardleServerOptions(db, logger)
	cmd := server.NewCommandStartWardleServer(ctx, options)

	return cli.Run(cmd)
}
