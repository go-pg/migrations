package migrations

import (
	"embed"
	"net/http"
	"testing"
)

//go:embed testdata/sqlmigrations/*.sql
var embedSQLMigrations embed.FS

func TestEmbedFS(t *testing.T) {
	DefaultCollection.DisableSQLAutodiscover(true)
	DefaultCollection.DiscoverSQLMigrationsFromFilesystem(http.FS(embedSQLMigrations), "/not-existing-dir")
	DefaultCollection.DiscoverSQLMigrationsFromFilesystem(http.FS(embedSQLMigrations), "/testdata/sqlmigrations")
	m := DefaultCollection.Migrations()
	if len(m) != 1 {
		t.Fatal("could not init migrations from filesystem")
	}
}
