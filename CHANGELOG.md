# Changelog

## v6.5

- `go run *.go init` must be used to create the `gopg_migrations` table.
- `gopg_migrations` table is locked during migration.
- Versions are updated within a PostgreSQL migration.

## v6.3

- Added support for pure SQL migrations located files `123_comment.up.sql` and `123_comment.down.sql`.
- Added `RegisterTx` and `MustRegisterTx` for migrations that must be run in transactions.
