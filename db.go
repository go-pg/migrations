package migrations

import (
	"strings"

	"gopkg.in/pg.v3"
)

var TableName = "gopg_migrations"

type DB interface {
	Exec(q string, args ...interface{}) (pg.Result, error)
	ExecOne(q string, args ...interface{}) (pg.Result, error)
	Query(f interface{}, q string, args ...interface{}) (pg.Result, error)
	QueryOne(model interface{}, q string, args ...interface{}) (pg.Result, error)
}

func Version(db DB) (int64, error) {
	if err := createTables(db); err != nil {
		return 0, err
	}

	var version int64
	_, err := db.QueryOne(pg.LoadInto(&version), `
		SELECT version FROM ? ORDER BY id DESC LIMIT 1
	`, pg.Q(TableName))
	if err != nil {
		if err == pg.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

func SetVersion(db DB, version int64) error {
	if err := createTables(db); err != nil {
		return err
	}

	_, err := db.Exec(`
		INSERT INTO ? (version, created_at) VALUES (?, now())
	`, pg.Q(TableName), version)
	return err
}

func createTables(db DB) error {
	if ind := strings.Index(TableName, "."); ind >= 0 {
		_, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ?`, pg.Q(TableName[:ind]))
		if err != nil {
			return err
		}
	}

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ? (
			id serial,
			version bigint,
			created_at timestamptz
		)
	`, pg.Q(TableName))
	return err
}
