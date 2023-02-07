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

Another change applied is setting the FillPercentage of the bucket.

By default, this value is 50%. Thanks to drand's workload type, we can optimize this and
change it to 100% as the workload is append-only.

This change is transparent to the user.


## Usage example

To use this tool, you should first stop the node, then run a command similar to this:
```shell
db-migration -beacon default -target bolt -source $TMP/d1/multibeacon/default/db/drand.db
```

This will produce a backup of the existing `drand.db` file
