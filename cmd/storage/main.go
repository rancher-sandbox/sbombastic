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
	"log"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rancher/sbombastic/cmd/storage/server"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"

	_ "modernc.org/sqlite"
)

var schema = `
CREATE TABLE IF NOT EXISTS sbom (
    key TEXT PRIMARY KEY,
    object BLOB NOT NULL
);
`

func main() {
	db, err := sqlx.Connect("sqlite", "storage.db")
	if err != nil {
		log.Fatalln(err)
	}

	db.MustExec(schema)

	ctx := genericapiserver.SetupSignalContext()
	options := server.NewWardleServerOptions(os.Stdout, os.Stderr, db)
	cmd := server.NewCommandStartWardleServer(ctx, options)
	code := cli.Run(cmd)
	os.Exit(code)
}
