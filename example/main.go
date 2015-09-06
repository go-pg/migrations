package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-pg/migrations"
	"gopkg.in/pg.v3"
)

const verbose = true

func main() {
	flag.Parse()

	db := pg.Connect(&pg.Options{
		User:     "postgres",
		Database: "pg_migrations_example",
	})

	oldVersion, newVersion, err := migrations.Run(db, flag.Args()...)
	if err != nil {
		exitf(err.Error())
	}
	if verbose && newVersion != oldVersion {
		fmt.Printf("migrated from version %d to %d\n", oldVersion, newVersion)
	}
}

func errorf(s string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, s+"\n", args...)
}

func exitf(s string, args ...interface{}) {
	errorf(s, args...)
	os.Exit(1)
}
