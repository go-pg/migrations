package main

import (
	"fmt"

	"gopkg.in/go-pg/migrations.v5"
)

func init() {
	migrations.Register(func(db migrations.DB) error {
		fmt.Println("seeding my_table...")
		_, err := db.Exec(`INSERT INTO my_table VALUES (1)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("truncating my_table...")
		_, err := db.Exec(`TRUNCATE my_table`)
		return err
	})
}
