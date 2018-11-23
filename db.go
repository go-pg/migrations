package migrations

import (
	"strings"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"github.com/go-pg/pg/types"
)

func SetTableName(name string) {
	DefaultCollection = DefaultCollection.WithTableName(name)
}

type DB = orm.DB

func (c *collection) getTableName() types.ValueAppender {
	return pg.Q(c.tableName)
}

func Version(db DB) (int64, error) {
	return DefaultCollection.Version(db)
}

func (c *collection) Version(db DB) (int64, error) {
	if err := c.createTables(db); err != nil {
		return 0, err
	}

	var version int64
	_, err := db.QueryOne(pg.Scan(&version), `
		SELECT version FROM ? ORDER BY id DESC LIMIT 1
	`, c.getTableName())
	if err != nil {
		if err == pg.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

func SetVersion(db DB, version int64) error {
	return DefaultCollection.SetVersion(db, version)
}

func (c *collection) SetVersion(db DB, version int64) error {
	if err := c.createTables(db); err != nil {
		return err
	}

	_, err := db.Exec(`
		INSERT INTO ? (version, created_at) VALUES (?, now())
	`, c.getTableName(), version)
	return err
}

func (c *collection) createTables(db DB) error {
	if ind := strings.IndexByte(c.tableName, '.'); ind >= 0 {
		_, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ?`, pg.Q(c.tableName[:ind]))
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
	`, c.getTableName())
	return err
}
