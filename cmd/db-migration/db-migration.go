package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/drand/drand-tools/internal/migration"
	"github.com/drand/drand/chain"
	"github.com/drand/drand/fs"
	"github.com/drand/drand/log"
)

var (
	sourcePath      = flag.String("source", "", "The source database to be migrated to the new format.")
	beaconName      = flag.String("beacon", "", "The name of the beacon to be migrated.")
	migrationTarget = flag.String("target", "", "The type of database to migrate to. Supported values: boltdb, postgres.\n"+
		"If boltdb is used, then the migration will be done in-place.\n"+
		"If postgres is used, then you need to specify the -pg-dsn flag value",
	)

	//nolint:lll // This is a flag
	pgDSN = flag.String("pg-dsn", `postgres://drand:drand@localhost:5432/drand?sslmode=disable&connect_timeout=5`, "The connection details for Postgres.")

	//nolint:lll // This is a flag
	bufferSize = flag.Int("buffer-size", migration.DefaultBufferSize, "Number of beacons that can be migrated at once. Use 0 to pre-allocate all the beacons.")
)

func main() {
	flag.Parse()
	logger := log.NewLogger(nil, log.LogDebug)

	err := run(logger)
	if err != nil && !errors.Is(err, migration.ErrMigrationNotNeeded) {
		logger.Panicw("while migrating the database", "err", err)
	}
	logger.Infow("finished migration process")
}

func run(logger log.Logger) error {
	err := checkValues(*sourcePath, *beaconName, *pgDSN, chain.StorageType(*migrationTarget))
	if err != nil {
		return fmt.Errorf("while validating input values: %w", err)
	}

	return migration.Migrate(logger, *sourcePath, *beaconName, chain.StorageType(*migrationTarget), *pgDSN, *bufferSize)
}

func checkValues(sourcePath, beaconName, pgDSN string, migrationTarget chain.StorageType) error {
	if sourcePath == "" {
		return fmt.Errorf("source path must be specified but %s", "source path is empty")
	}

	if beaconName == "" {
		return fmt.Errorf("beacon name must be specified but %s", "beacon name is empty")
	}

	if e, _ := fs.Exists(sourcePath); !e {
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
