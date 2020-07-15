package migrations

import (
	"context"
	"io"

	"github.com/go-pg/pg/v10"
	"github.com/go-pg/pg/v10/orm"
)

// DB is a common interface for pg.DB and pg.Tx types.
type DB interface {
	Model(model ...interface{}) *orm.Query

	Exec(query interface{}, params ...interface{}) (orm.Result, error)
	ExecOne(query interface{}, params ...interface{}) (orm.Result, error)
	Query(coll, query interface{}, params ...interface{}) (orm.Result, error)
	QueryOne(model, query interface{}, params ...interface{}) (orm.Result, error)

	Begin() (*pg.Tx, error)

	CopyFrom(r io.Reader, query interface{}, params ...interface{}) (orm.Result, error)
	CopyTo(w io.Writer, query interface{}, params ...interface{}) (orm.Result, error)

	Context() context.Context
}
