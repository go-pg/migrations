# Example

You need to create database `pg_migrations_example` before running this example.

```
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
migrated from version 0 to 3

> go run *.go version
version is 3

> go run *.go create add_email_to_users
created migration file: 4_add_email_to_users.go

> go run *.go down
truncating my_table...
migrated from version 3 to 2

> go run *.go version
version is 2

> go run *.go set_version 1
migrated from version 2 to 1
```
