package migration

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"

	json "github.com/nikkolasg/hexjson"
	"go.etcd.io/bbolt"

	"github.com/drand/drand/chain"
	"github.com/drand/drand/chain/postgresdb/database"
	"github.com/drand/drand/chain/postgresdb/pgdb"
	"github.com/drand/drand/chain/postgresdb/schema"
	"github.com/drand/drand/log"
)

type (
	beacon struct {
		PreviousSig []byte `json:",omitempty"`
		Round       uint64
		Signature   []byte
	}

	migrator struct {
		startedAt time.Time
		logger    log.Logger

		pgDSN string
		// Controls different buffers depending on the storageTargetType:
		// - For BoltDB, this keeps all entries without committing them to the disk.
		// - For Postgres, this is automatically capped 30_000 entries to due limitations in Postgres.
		bufferSize       int
		beaconName       string
		sourceBeaconPath string

		existingRows      int
		migratedRows      int
		storageTargetType chain.StorageType
	}
)

const (
	ownerOnly = 0600

	DefaultBufferSize = 10_000
)

var (
	// ErrMigrationNotNeeded is returned if the database format is already at the target version
	ErrMigrationNotNeeded = errors.New("migration not needed")

	bucketName = []byte("beacons")
)

// Migrate runs the migration based on the input parameters.
//
//nolint:lll // This is intended
func Migrate(logger log.Logger, sourceBeaconPath, beaconName string, storageTargetType chain.StorageType, pgDSN string, bufferSize int) error {
	startedAt := time.Now()

	if err := shouldMigrate(logger, sourceBeaconPath, beaconName, storageTargetType, pgDSN); err != nil {
		if errors.Is(err, ErrMigrationNotNeeded) {
			logger.Warnw("decided storage format migration is not needed", "err", err)
		}
		return err
	}

	bufferSize, err := computerBufferSize(bufferSize, logger, sourceBeaconPath)
	if err != nil {
		return err
	}

	m := migrator{
		logger:    logger,
		startedAt: startedAt,

		pgDSN:             pgDSN,
		bufferSize:        bufferSize,
		beaconName:        beaconName,
		sourceBeaconPath:  sourceBeaconPath,
		storageTargetType: storageTargetType,
	}

	return m.doMigrate()
}

func computerBufferSize(bufferSize int, logger log.Logger, sourceBeaconPath string) (int, error) {
	if bufferSize < 0 {
		logger.Infow("buffer size not specified, defaulting to 10000")
		return DefaultBufferSize, nil
	}

	if bufferSize == 0 {
		return automaticBufferSize(logger, sourceBeaconPath)
	}

	if bufferSize < DefaultBufferSize {
		logger.Warnw("buffer size seems a bit too small. The migration process might be slow", "bufferSize", bufferSize)
	}

	//nolint:gomnd // See below
	if bufferSize > 10_000_000 {
		//nolint:lll // This line has the right amount of chars
		logger.Warnw("buffer size seems a bit too large. Make sure your system can allocate enough system memory for this", "bufferSize", bufferSize)
	}
	return bufferSize, nil
}

func automaticBufferSize(logger log.Logger, sourceBeaconPath string) (int, error) {
	logger.Warnw("buffer size set to 0. Running automatic buffer inference. Make sure you have enough system memory for this")
	existingDB, err := bbolt.Open(sourceBeaconPath, ownerOnly, nil)
	if err != nil {
		return -1, err
	}
	defer existingDB.Close()

	var bufferSize = 0

	err = existingDB.View(func(tx *bbolt.Tx) error {
		bufferSize = tx.Bucket(bucketName).Stats().KeyN
		return nil
	})
	if err != nil {
		return 0, err
	}

	logger.Infow("buffer size automatically inferred from existing DB", "bufferSize", bufferSize)

	return bufferSize, nil
}

func shouldMigrate(logger log.Logger, sourceBeaconPath, beaconName string, destination chain.StorageType, pgDSN string) error {
	//nolint:exhaustive // We want to explicitly ignore the chain.MemDB backend since there's nothing to migrate there.
	switch destination {
	case chain.BoltDB:
		return shouldMigrateBolt(sourceBeaconPath)
	case chain.PostgreSQL:
		return shouldMigratePostgres(logger, beaconName, pgDSN)
	default:
		return fmt.Errorf("unknown migration storage target type %q for migration package", destination)
	}
}

func shouldMigrateBolt(sourceBeaconPath string) error {
	existingDB, err := bbolt.Open(sourceBeaconPath, ownerOnly, nil)
	if err != nil {
		return err
	}
	defer existingDB.Close()

	return existingDB.View(func(tx *bbolt.Tx) error {
		_, value := tx.Bucket(bucketName).Cursor().First()
		var b = beacon{}
		if json.Unmarshal(value, &b) != nil {
			return ErrMigrationNotNeeded
		}

		return nil
	})
}

func pgConn(ctx context.Context, logger log.Logger, beaconName, pgDSN string) (*pgdb.Store, func(), error) {
	pgConf, err := database.ConfigFromDSN(pgDSN)
	if err != nil {
		return nil, nil, err
	}

	conn, err := database.Open(ctx, pgConf)
	if err != nil {
		return nil, nil, err
	}

	if err := schema.Migrate(ctx, conn); err != nil {
		err := fmt.Errorf("migrating error: %w", err)
		return nil, nil, err
	}

	store, err := pgdb.NewStore(ctx, logger, conn, beaconName)
	if err != nil {
		return nil, nil, err
	}

	cancel := func() {
		store.Close(ctx)
		conn.Close()
	}

	return store, cancel, err
}

func shouldMigratePostgres(logger log.Logger, beaconName, pgDSN string) error {
	ctx := context.Background()

	store, cancel, err := pgConn(ctx, logger, beaconName, pgDSN)
	if err != nil {
		return err
	}
	defer cancel()

	storeLen, err := store.Len(ctx)
	if err != nil {
		return err
	}

	if storeLen != 0 {
		logger.Warnw("Postgres contains beacon rounds. Skipping migration", "rounds count", storeLen)
		return ErrMigrationNotNeeded
	}

	return nil
}

func (m *migrator) doMigrate() (retErr error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()

		finishedIn := time.Since(m.startedAt).String()

		if m.existingRows != m.migratedRows {
			errMessage := "not all rounds migrated successfully expected: %d actual: %d\nconsider running drand sync to restore the missing rounds"
			retErr = fmt.Errorf(errMessage, m.existingRows, m.migratedRows)
			return
		}

		m.logger.Infow(
			"Finished processing beacons",
			"beaconName", m.beaconName,
			"finishedIn", finishedIn,
		)
	}()

	//nolint:exhaustive // We want to explicitly ignore the chain.MemDB backend since there's nothing to migrate there.
	switch m.storageTargetType {
	case chain.BoltDB:
		return m.migrateBolt(ctx)
	case chain.PostgreSQL:
		return m.migratePostgres(ctx)
	default:
		return nil
	}
}

func (m *migrator) migratePostgres(ctx context.Context) (retErr error) {
	store, cancel, err := pgConn(ctx, m.logger, m.beaconName, m.pgDSN)
	if err != nil {
		return err
	}

	defer cancel()

	m.logger.Infow("dropping the FK from the table")
	err = store.DropFK(ctx)
	if err != nil {
		return err
	}
	defer func() {
		m.logger.Infow("adding FK back to the table")
		err := store.AddFK(ctx)
		if err != nil {
			retErr = err
			return
		}
	}()

	// Make sure we can still commit to the database avoiding the error:
	//   pq: got 9229389 parameters but PostgreSQL only supports 65535 parameters
	const maxPgBufferSize = 30_000

	pgBuffSize := m.bufferSize
	if pgBuffSize > maxPgBufferSize {
		pgBuffSize = maxPgBufferSize
		m.logger.Warnw("buffer size automatically reconfigured for Postgres only", "bufferSize", pgBuffSize)
	}

	buffSize := 0
	bs := make([]chain.Beacon, pgBuffSize)

	err = m.reader(func(val beacon) error {
		m.migratedRows++
		b := chain.Beacon(val)

		bs[buffSize] = b
		buffSize++

		if buffSize == pgBuffSize {
			err := store.BatchPut(ctx, bs)
			if err != nil {
				m.logger.Errorw("while writing buffer contents to DB", "err", err)
				return err
			}

			buffSize = 0
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Check to see if there are any elements left unsaved in the buffer
	if buffSize > 0 {
		m.logger.Debugw("writing buffer contents to DB", "rows", buffSize)
		err := store.BatchPut(ctx, bs[:buffSize])
		if err != nil {
			m.logger.Errorw("while writing buffer contents to DB", "err", err)
			return err
		}
	}

	return nil
}

func (m *migrator) swapMigratedFile(newBeaconPath string) error {
	if m.existingRows != m.migratedRows {
		return fmt.Errorf(
			"abort swapping migrated files. Not all rounds migrated successfully expected: %d actual: %d",
			m.existingRows,
			m.migratedRows,
		)
	}

	m.logger.Infow("migrating BoltDB file")

	err := os.Rename(m.sourceBeaconPath, fmt.Sprintf("%s.old", m.sourceBeaconPath))
	if err != nil {
		return err
	}

	return os.Rename(newBeaconPath, m.sourceBeaconPath)
}

func (m *migrator) migrateBolt(ctx context.Context) error {
	newBeaconPath := m.sourceBeaconPath + "-migrated"

	db, err := bbolt.Open(newBeaconPath, ownerOnly, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	db.MaxBatchSize = m.bufferSize

	err = db.Batch(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(bucketName)
		if err != nil {
			return err
		}

		// We know this will be an append-only workload, it's safe to do it this way.
		bucket.FillPercent = 1.0

		//nolint:gomnd // uint64 is 8 bytes
		newKey := make([]byte, 8)

		return m.reader(func(val beacon) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			m.migratedRows++

			binary.BigEndian.PutUint64(newKey, val.Round)
			return bucket.Put(newKey, val.Signature)
		})
	})

	if err != nil {
		return err
	}

	return m.swapMigratedFile(newBeaconPath)
}

func (m *migrator) reader(callback func(beacon) error) error {
	defer func() {
		finishedIn := time.Since(m.startedAt).String()
		m.logger.Infow(
			"finished reading existing beacon database",
			"beaconName", m.beaconName,
			"rows", m.existingRows,
			"finishedIn", finishedIn,
		)
	}()

	existingDB, err := bbolt.Open(m.sourceBeaconPath, ownerOnly, nil)
	if err != nil {
		return err
	}

	defer existingDB.Close()

	return existingDB.View(func(tx *bbolt.Tx) error {
		existingBucket := tx.Bucket(bucketName)
		return existingBucket.ForEach(func(k, v []byte) error {
			m.existingRows++

			b := beacon{}
			err := json.Unmarshal(v, &b)
			if err != nil {
				return err
			}

			return callback(b)
		})
	})
}
