# Drand database migration tool


This tool migrates the database format from the version used prior to Drand v1.5.2 to the newer format.

If you used Drand before v1.5.2, then you should keep reading forward.
If you are new to using Drand, or started with v1.5.2, then you can skip reading this.

## Database Changes

Before the v1.5.2, BoltDB database had the following format/contents:
- key -> base64 encoding of the round number
- value -> hex encoded json value of the beacon containing `previous_sig`, `round`, and `signature` fields

After applying this update, the following changes will be applied to the database format/contents:
- key -> base64 encoding of the round number
- value -> `signature` stored as raw bytes

This change is a breaking change. Once migrated, you won't be able to use the old drand version with the
database format.

Another change applied is setting the FillPercentage of the BoltDB bucket.

By default, this value is 50%. Thanks to drand's workload type, we can optimize this and
change it to 100% as the workload is append-only.

This change is transparent to the user.


## Usage example

To use this tool, you should first stop the node. Then, run the command with the desired target type.


### BoltDB to BoltDB

You'll need to:
1. stop the drand node
2. copy the database files from its multibeacon folder for the beacons you're running to somewhere else
3. restart the node to make sure it's up during the update of the databases
4. process all your database copies using this tool, creating the new ones
5. stop the node again
6. put the new format database instead of the old one
7. restart the node and let it sync with the rest of the network (since the copy of the database will be missing the last few beacons)

To migrate from the pre-1.5 BoltDB storage format to the new one, run a command similar to this:
```shell
db-migration -beacon default -target bolt -buffer-size 20000 -source $TMP/d1/multibeacon/default/db/drand.db
```

This will:
- Use the `source` file as an input to migrate from that database format.
- Swap the newly created file with the `source` file.
- Produce an in-place backup of the existing `drand.db` file.

The backup is produced only when migrating from BoltDB to BoltDB. In the above example,
the backup file will be under `$TMP/d1/multibeacon/default/db/drand.db.old`.
A backup will not be produced for a migration from BoltDB to PostgreSQL.

### BoltDB to PostgreSQL

To migrate from BoltDB to PostgreSQL, run a command similar to this:

```shell
db-migration -beacon default -target postgres -pg-dsn 'postgres://user:password@host:port/drand?sslmode=disable&connect_timeout=5' -buffer-size 20000 -source $TMP/d1/multibeacon/default/db/drand.db 
```

This will:
- Create the required database tables.
- Apply any migration differences between existing tables, created by a previous run of this tool, and new ones.
- Start performing the migration from BoltDB to Postgres

### Buffering size

To help with different system constraints, the tool exposes the `-buffer-size` flag.

This allows users to set the number of beacons stored in-memory during the migration. It will also
act as a hint for how large the BoltDB transaction must be before flushing the records to the file.

The logic for setting it is:
- If the buffer-size < 0 then the application will default to 10000 entries.
- If the buffer-size == 0 then the application will use the number of rounds stored in the source database as size. For large workloads, this might consume a lot of memory.
- If the is 0 < buffer-size < 10000 then you'll see a warning about the process possibly taking a bit longer than usual.
- If the buffer-size > 10000000 then you'll see a warning about this as the system memory could get pretty high.
