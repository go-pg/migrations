package migrations

import (
	"embed"
	"net/http"
	"testing"
)

//go:embed testdata/sqlmigrations/*.sql
var embedSQLMigrations embed.FS

func TestEmbedFS(t *testing.T) {
	coll := NewCollection()
	coll.DisableSQLAutodiscover(true)
	coll.DiscoverSQLMigrationsFromFilesystem(http.FS(embedSQLMigrations), "not-existing-dir")
	coll.DiscoverSQLMigrationsFromFilesystem(http.FS(embedSQLMigrations), "testdata/sqlmigrations")
	m := coll.Migrations()
	if len(m) != 1 {
		t.Fatal("could not init migrations from filesystem")
	}
}
