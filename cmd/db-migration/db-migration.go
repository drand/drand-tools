package main

import (
	"context"
	"errors"
	"flag"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/log"

	"github.com/drand/drand-tools/internal/migration"
)

var buildDate string
var gitCommit string

var sourcePath = flag.String("source", "", "The source database to be migrated to the new format.")
var beaconName = flag.String("beacon", "", "The name of the beacon to be migrated.")
var migrationTarget = flag.String("target", "", "The type of database to migrate to. Supported values: boltdb, postgres.\n"+
	"If boltdb is used, then the migration will be done in-place.\n"+
	"If postgres is used, then you need to specify the -pg-dsn flag value",
)
var pgDSN = flag.String("pg-dsn", `postgres://drand:drand@localhost:5432/drand?sslmode=disable&connect_timeout=5`, "The connection details for Postgres.")

func main() {
	flag.Parse()
	logger := log.NewLogger(nil, log.LogDebug)

	if *sourcePath == "" {
		logger.Fatalw("source path must be specified", "err", "source path is empty")
	}

	if *beaconName == "" {
		logger.Fatalw("source path must be specified", "err", "source path is empty")
	}

	if e, _ := fs.Exists(*sourcePath); e {
		logger.Fatalw("source path must be specified", "err", "source path does not exists")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if chain.StorageType(*migrationTarget) != chain.BoltDB &&
		chain.StorageType(*migrationTarget) != chain.PostgreSQL {
		logger.Panicw("migration type is not support.", "target", migrationTarget)
	}

	err := migration.Migrate(ctx, logger, *sourcePath, *beaconName, chain.StorageType(*migrationTarget), *pgDSN, 0)
	if err != nil &&
		!errors.Is(err, migration.ErrMigrationNotNeeded) {
		logger.Panicw("while migrating the databaase", "err", err)
	}
	logger.Infow("finished migration process")
	_ = buildDate
	_ = gitCommit
}
