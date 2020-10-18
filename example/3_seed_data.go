package main

import (
	"context"
	"fmt"

	"github.com/go-pg/migrations/v9"
)

func init() {
	migrations.MustRegisterTx(func(ctx context.Context, db migrations.DB) error {
		fmt.Println("seeding my_table...")
		_, err := db.Exec(ctx, `INSERT INTO my_table VALUES (1)`)
		return err
	}, func(ctx context.Context, db migrations.DB) error {
		fmt.Println("truncating my_table...")
		_, err := db.Exec(ctx, `TRUNCATE my_table`)
		return err
	})
}
