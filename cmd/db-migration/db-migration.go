package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/drand/drand-tools/internal/migration"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/log"
)

var sourcePath = flag.String("source", "", "The source database to be migrated to the new format.")
var beaconName = flag.String("beacon", "", "The name of the beacon to be migrated.")
var migrationTarget = flag.String("target", "", "The type of database to migrate to. Supported values: boltdb, postgres.\n"+
	"If boltdb is used, then the migration will be done in-place.\n"+
	"If postgres is used, then you need to specify the -pg-dsn flag value",
)

//nolint:lll // This is a flag
var pgDSN = flag.String("pg-dsn", `postgres://drand:drand@localhost:5432/drand?sslmode=disable&connect_timeout=5`, "The connection details for Postgres.")
var bufferSize = flag.Int("buffer-size", 10_000, "Number of beacons that can be migrated at once. Use 0 to pre-allocate all the beacons.")

func main() {
	flag.Parse()
	logger := log.NewLogger(nil, log.LogDebug)

	err := checkValues(*sourcePath, *beaconName, *pgDSN, chain.StorageType(*migrationTarget))
	if err != nil {
		logger.Panicw("while validating input values", "err", err)
	}

	err = migrate(logger, *sourcePath, *beaconName, *pgDSN, chain.StorageType(*migrationTarget), *bufferSize)
	if err != nil &&
		!errors.Is(err, migration.ErrMigrationNotNeeded) {
		logger.Panicw("while migrating the databaase", "err", err)
	}
	logger.Infow("finished migration process")
}

func checkValues(sourcePath, beaconName, pgDSN string, migrationTarget chain.StorageType) error {
	if sourcePath == "" {
		return fmt.Errorf("source path must be specified but %s", "source path is empty")
	}

	if beaconName == "" {
		return fmt.Errorf("beacon name must be specified but %s", "beacon name is empty")
	}

	if e, _ := fs.Exists(sourcePath); e {
		return fmt.Errorf("source path must be specified but %s", "source path does not exists")
	}

	if migrationTarget != chain.BoltDB &&
		migrationTarget != chain.PostgreSQL {
		return fmt.Errorf("unknown migration target %s", migrationTarget)
	}

	if pgDSN == "" {
		return fmt.Errorf("postgres dsn is empty")
	}

	return nil
}

func migrate(logger log.Logger, sourcePath, beaconName, pgDSN string, migrationTarget chain.StorageType, bufferSize int) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return migration.Migrate(ctx, logger, sourcePath, beaconName, migrationTarget, pgDSN, bufferSize)
}
