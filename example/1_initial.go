package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	fmt.Println("1_initial.go: init function is called")
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("creating table my_table...")
		_, err := db.Exec(`CREATE TABLE my_table()`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("dropping table my_table...")
		_, err := db.Exec(`DROP TABLE my_table`)
		return err
	})
}
