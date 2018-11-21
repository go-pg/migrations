package migrations

import (
	"strings"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"github.com/go-pg/pg/types"
)

var defaultTableName = "gopg_migrations"

func SetTableName(name string) {
	defaultTableName = name
}

func (g *Group) tableName() string {
	if g.TableName == "" {
		return defaultTableName
	}
	return g.TableName
}

type DB = orm.DB

func (g *Group) getTableName() types.ValueAppender {
	return pg.Q(g.tableName())
}

func Version(db DB) (int64, error) {
	return DefaultGroup.Version(db)
}

func (g *Group) Version(db DB) (int64, error) {
	if err := g.createTables(db); err != nil {
		return 0, err
	}

	var version int64
	_, err := db.QueryOne(pg.Scan(&version), `
		SELECT version FROM ? ORDER BY id DESC LIMIT 1
	`, g.getTableName())
	if err != nil {
		if err == pg.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

func SetVersion(db DB, version int64) error {
	return DefaultGroup.SetVersion(db, version)
}

func (g *Group) SetVersion(db DB, version int64) error {
	if err := g.createTables(db); err != nil {
		return err
	}

	_, err := db.Exec(`
		INSERT INTO ? (version, created_at) VALUES (?, now())
	`, g.getTableName(), version)
	return err
}

func (g *Group) createTables(db DB) error {
	if ind := strings.IndexByte(g.tableName(), '.'); ind >= 0 {
		_, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ?`, pg.Q(g.tableName()[:ind]))
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
	`, g.getTableName())
	return err
}
