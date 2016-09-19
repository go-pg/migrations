package migrations

import (
	"strings"

	"gopkg.in/pg.v5"
	"gopkg.in/pg.v5/orm"
	"gopkg.in/pg.v5/types"
)

var tableName = "gopg_migrations"

func SetTableName(name string) {
	tableName = name
}

type DB interface {
	Exec(query interface{}, params ...interface{}) (*types.Result, error)
	ExecOne(query interface{}, params ...interface{}) (*types.Result, error)
	Query(model, query interface{}, params ...interface{}) (*types.Result, error)
	QueryOne(model, query interface{}, params ...interface{}) (*types.Result, error)
	Model(...interface{}) *orm.Query
	FormatQuery(dst []byte, query string, params ...interface{}) []byte
}

func getTableName() orm.FormatAppender {
	return pg.Q(tableName)
}

func Version(db DB) (int64, error) {
	var version int64
	_, err := db.QueryOne(pg.Scan(&version), `
		SELECT version FROM ? ORDER BY id DESC LIMIT 1
	`, getTableName())
	if err != nil {
		if err == pg.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

func SetVersion(db DB, version int64) error {
	_, err := db.Exec(`
		INSERT INTO ? (version, created_at) VALUES (?, now())
	`, getTableName(), version)
	return err
}

func createTables(db DB) error {
	if ind := strings.IndexByte(tableName, '.'); ind >= 0 {
		_, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ?`, tableName[:ind])
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
	`, getTableName())
	return err
}
