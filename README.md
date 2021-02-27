# SQL migrations for Golang and PostgreSQL

[![Build Status](https://travis-ci.org/go-pg/migrations.svg)](https://travis-ci.org/go-pg/migrations)
[![GoDoc](https://godoc.org/github.com/go-pg/migrations?status.svg)](https://godoc.org/github.com/go-pg/migrations)

This package allows you to run migrations on your PostgreSQL database using [Golang Postgres client](https://github.com/go-pg/pg). See [example](example) for details.

You may also want to check [go-pg-migrations](https://github.com/robinjoseph08/go-pg-migrations) before making a decision.

# Installation

go-pg/migrations requires a Go version with [Modules](https://github.com/golang/go/wiki/Modules) support and uses import path versioning. So please make sure to initialize a Go module:

```shell
go mod init github.com/my/repo
go get github.com/go-pg/migrations/v8
```

# Usage

To run migrations on your project you should fulfill the following steps:

1. define the migration list;
1. implement an executable app that calls migration tool;
1. run migrations.

## Define Migrations

### Migration Files

You can save SQL migration files at the same directory as your `main.go` file, they should have proper file extensions ([more about migration files](#sql-migrations)).

### Registered Migrations

Migrations can be registered in the code using `migrations.RegisterTx` and `migrations.MustRegisterTx` functions. [More details](#registering-migrations) about migration registering.

## Implement app to run the tool

You can run migrations from any place of your app or ecosystem. It can be a standalone application of a part of a big program, or maybe an HTTP handler, etc. Check [example](#example) for some helpful information about practical usage.

## Run Migrations

Run migration tool by providing CLI arguments to the `migrations.Run` function.

Currently, the following arguments are supported:

- `up` - runs all available migrations;
- `up [target]` - runs available migrations up to the target one;
- `down` - reverts last migration;
- `reset` - reverts all migrations;
- `version` - prints current db version;
- `set_version [version]` - sets db version without running migrations.

# Example

You need to create database `pg_migrations_example` before running the [example](example).

```bash
> cd example

> psql -c "CREATE DATABASE pg_migrations_example"
CREATE DATABASE

> go run *.go init
version is 0

> go run *.go version
version is 0

> go run *.go
creating table my_table...
adding id column...
seeding my_table...
migrated from version 0 to 4

> go run *.go version
version is 4

> go run *.go reset
truncating my_table...
dropping id column...
dropping table my_table...
migrated from version 4 to 0

> go run *.go up 2
creating table my_table...
adding id column...
migrated from version 0 to 2

> go run *.go
seeding my_table...
migrated from version 2 to 4

> go run *.go down
truncating my_table...
migrated from version 4 to 3

> go run *.go version
version is 3

> go run *.go set_version 1
migrated from version 3 to 1

> go run *.go create add email to users
created migration 5_add_email_to_users.go
```

## Registering Migrations

### `migrations.RegisterTx` and `migrations.MustRegisterTx`

Registers migrations to be executed inside transactions.

### `migrations.Register` and `migrations.MustRegister`

Registers migrations to be executed without any transaction.

## SQL migrations

SQL migrations are automatically picked up if placed in the same folder with `main.go` or Go migrations.
SQL migrations may be manually registered using `DiscoverSQLMigrations` (from OS directory) or `DiscoverSQLMigrationsFromFilesystem`.
It may be used with new go 1.16 embedding feature. Example:
```go
//go:embed migrations/*.sql
var migrations embed.FS

collection := migrations.NewCollection()
collection.DiscoverSQLMigrationsFromFilesystem(http.FS(migrations), "migrations")
```
SQL migrations must have one of the following extensions:

- .up.sql - up migration;
- .down.sql - down migration;
- .tx.up.sql - transactional up migration;
- .tx.down.sql - transactional down migration.

By default SQL migrations are executed as single PostgreSQL statement. `--gopg:split` directive can be used to split migration into several statements:

```sql
SET statement_timeout = 60000;
SET lock_timeout = 60000;

--gopg:split

CREATE INDEX CONCURRENTLY ...;
```

## Transactions

By default, the migrations are executed outside without any transactions. Individual migrations can however be marked to be executed inside transactions by using the `RegisterTx` function instead of `Register`.

### Global Transactions

```go
var oldVersion, newVersion int64

err := db.RunInTransaction(func(tx *pg.Tx) (err error) {
    oldVersion, newVersion, err = migrations.Run(tx, flag.Args()...)
    return
})
if err != nil {
    exitf(err.Error())
}
```
