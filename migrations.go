package migrations

import (
	"context"
	"io"

	"github.com/go-pg/pg/v11"
	"github.com/go-pg/pg/v11/orm"
)

// DB is a common interface for pg.DB and pg.Tx types.
type DB interface {
	Model(model ...interface{}) *orm.Query

	Exec(ctx context.Context, query interface{}, params ...interface{}) (orm.Result, error)
	ExecOne(ctx context.Context, query interface{}, params ...interface{}) (orm.Result, error)
	Query(ctx context.Context, coll, query interface{}, params ...interface{}) (orm.Result, error)
	QueryOne(ctx context.Context, model, query interface{}, params ...interface{}) (orm.Result, error)

	Begin(ctx context.Context) (*pg.Tx, error)

	CopyFrom(ctx context.Context, r io.Reader, query interface{}, params ...interface{}) (orm.Result, error)
	CopyTo(ctx context.Context, w io.Writer, query interface{}, params ...interface{}) (orm.Result, error)
}
