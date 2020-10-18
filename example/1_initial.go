package main

import (
	"context"
	"fmt"

	"github.com/go-pg/migrations/v9"
)

func init() {
	migrations.MustRegisterTx(func(ctx context.Context, db migrations.DB) error {
		fmt.Println("creating table my_table...")
		_, err := db.Exec(ctx, `CREATE TABLE my_table()`)
		return err
	}, func(ctx context.Context, db migrations.DB) error {
		fmt.Println("dropping table my_table...")
		_, err := db.Exec(ctx, `DROP TABLE my_table`)
		return err
	})
}
