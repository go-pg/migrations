package main

import (
	"context"
	"fmt"

	"github.com/go-pg/migrations/v9"
)

func init() {
	migrations.MustRegisterTx(func(ctx context.Context, db migrations.DB) error {
		fmt.Println("adding id column...")
		_, err := db.Exec(ctx, `ALTER TABLE my_table ADD id serial`)
		return err
	}, func(ctx context.Context, db migrations.DB) error {
		fmt.Println("dropping id column...")
		_, err := db.Exec(ctx, `ALTER TABLE my_table DROP id`)
		return err
	})
}
