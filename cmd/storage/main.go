/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rancher/sbombastic/internal/database/sqlc"
	"github.com/rancher/sbombastic/internal/storage"

	"github.com/rancher/sbombastic/cmd/storage/server"
	"github.com/rancher/sbombastic/internal/database"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	_ "modernc.org/sqlite"
)

func run(logger *slog.Logger) error {
	safeExit := 0
	ctx := genericapiserver.SetupSignalContext()
	pool, err := database.NewPGX(ctx)
	if err != nil {
		return fmt.Errorf("new pgx: %w", err)
	}
	defer pool.Close()

	queries := sqlc.New(pool)
	if err = queries.CreateImageTable(ctx); err != nil {
		return fmt.Errorf("create image  table: %w", err)
	}
	if err = queries.CreateSbomTable(ctx); err != nil {
		return fmt.Errorf("create sbom table: %w", err)
	}
	if err = queries.CreateVulnerabilityReportsTable(ctx); err != nil {
		return fmt.Errorf("create vulnerability reports table: %w", err)
	}

	options := server.NewWardleServerOptions(pool, logger)
	cmd := server.NewCommandStartWardleServer(ctx, options)
	code := cli.Run(cmd)
	if code != safeExit {
		return fmt.Errorf("cli exited with code %d", code)
	}
	return nil
}

func main() {
	// TODO: add CLI flags
	opts := slog.HandlerOptions{
		Level: slog.LevelDebug,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &opts)).With("component", "storage")

	db, err := sqlx.Connect("sqlite", "/data/sqlite/storage.db")
	if err != nil {
		log.Fatalln(err)
	}

	db.SetMaxOpenConns(1) // Avoid connection pool issues
	db.MustExec(storage.CreateImageTableSQL)
	db.MustExec(storage.CreateSBOMTableSQL)
	db.MustExec(storage.CreateVulnerabilityReportTableSQL)

	if err := run(logger); err != nil {
		log.Fatalln(err)
	}
}
