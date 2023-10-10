package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	fmt.Println("2_add_id.go: init function is called!!!!")
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("adding id column...")
		_, err := db.Exec(`ALTER TABLE my_table ADD id serial`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("dropping id column...")
		_, err := db.Exec(`ALTER TABLE my_table DROP id`)
		return err
	})
}
