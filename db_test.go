package migrations_test

import (
	"fmt"
	"github.com/go-pg/migrations"
	"testing"

	"github.com/go-pg/pg"
)

func connectDB() *pg.DB {
	db := pg.Connect(&pg.Options{
		User: "postgres",
	})

	_, err := db.Exec("DROP TABLE IF EXISTS gopg_migrations")
	if err != nil {
		panic(err)
	}

	_, _, err = migrations.Run(db, "init")
	if err != nil {
		panic(err)
	}

	return db
}

func TestVersion(t *testing.T) {
	db := connectDB()

	version, err := migrations.Version(db)
	if err != nil {
		t.Fatalf("Version failed: %s", err)
	}
	if version != 0 {
		t.Fatalf("got version %d, wanted 0", version)
	}

	if err := migrations.SetVersion(db, 999); err != nil {
		t.Fatalf("SetVersion failed: %s", err)
	}

	version, err = migrations.Version(db)
	if err != nil {
		t.Fatalf("Version failed: %s", err)
	}
	if version != 999 {
		t.Fatalf("got version %d, wanted 999", version)
	}
}

func TestUpDown(t *testing.T) {
	db := connectDB()

	coll := migrations.NewCollection([]*migrations.Migration{
		{Version: 2, Up: doNothing, Down: doNothing},
		{Version: 1, Up: doNothing, Down: doNothing},
		{Version: 3, Up: doNothing, Down: doNothing},
	}...)
	oldVersion, newVersion, err := coll.Run(db, "up")
	if err != nil {
		t.Fatal(err)
	}
	if oldVersion != 0 {
		t.Fatalf("got %d, wanted 0", oldVersion)
	}
	if newVersion != 3 {
		t.Fatalf("got %d, wanted 3", newVersion)
	}

	version, err := coll.Version(db)
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("got version %d, wanted 3", version)
	}

	for i := 2; i >= -5; i-- {
		wantOldVersion := int64(i + 1)
		wantNewVersion := int64(i)
		if wantNewVersion < 0 {
			wantOldVersion = 0
			wantNewVersion = 0
		}

		oldVersion, newVersion, err = coll.Run(db, "down")
		if err != nil {
			t.Fatal(err)
		}
		if oldVersion != wantOldVersion {
			t.Fatalf("got %d, wanted %d", oldVersion, wantOldVersion)
		}
		if newVersion != wantNewVersion {
			t.Fatalf("got %d, wanted %d", newVersion, wantNewVersion)
		}

		version, err = coll.Version(db)
		if err != nil {
			t.Fatal(err)
		}
		if version != wantNewVersion {
			t.Fatalf("got version %d, wanted %d", version, wantNewVersion)
		}
	}
}

func TestSetVersion(t *testing.T) {
	db := connectDB()

	coll := migrations.NewCollection([]*migrations.Migration{
		{Version: 3, Up: doPanic, Down: doPanic},
		{Version: 1, Up: doPanic, Down: doPanic},
		{Version: 2, Up: doPanic, Down: doPanic},
	}...)

	for i := 0; i < 5; i++ {
		wantOldVersion := int64(i)
		wantNewVersion := int64(i + 1)

		oldVersion, newVersion, err := coll.Run(
			db, "set_version", fmt.Sprint(wantNewVersion))
		if err != nil {
			t.Fatal(err)
		}
		if oldVersion != wantOldVersion {
			t.Fatalf("got %d, wanted %d", oldVersion, wantOldVersion)
		}
		if newVersion != wantNewVersion {
			t.Fatalf("got %d, wanted %d", newVersion, wantNewVersion)
		}

		version, err := coll.Version(db)
		if err != nil {
			t.Fatal(err)
		}
		if version != wantNewVersion {
			t.Fatalf("got version %d, wanted %d", version, wantNewVersion)
		}
	}
}

func TestSetSchema(t *testing.T) {

	cases := []struct {
		desc           string
		targetSchema   string
		expectedSchema string
		expectedTable  string
	}{
		{"default schema", "", "public", "gopg_migrations"},
		{"set specific schema", "testschema", "testschema", "gopg_migrations"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {

			db := pg.Connect(&pg.Options{
				User: "postgres",
			})

			expectedSchema := tc.expectedSchema
			expectedTable := tc.expectedTable

			coll := migrations.DefaultCollection
			if tc.targetSchema != "" {
				coll.SetSchemaName(tc.targetSchema)
			}

			_, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", expectedSchema, expectedTable))
			if err != nil {
				t.Fatalf("error dropping table: %s", err.Error())
			}

			_, _, err = coll.Run(db, "init")
			if err != nil {
				t.Fatalf("problem in init: %s", err.Error())
			}

			n, err := db.Model().
				Table("pg_tables").
				Where("schemaname = '?'", pg.Q(expectedSchema)).
				Where("tablename = '?'", pg.Q(expectedTable)).
				Count()

			if n != 1 {
				t.Fatalf("did not find expected table: %s.%s", expectedSchema, expectedTable)
			}
		})
	}
}

func doNothing(db migrations.DB) error {
	return nil
}

func doPanic(db migrations.DB) error {
	panic("this migration should not be run")
}
