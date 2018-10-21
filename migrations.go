package migrations

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/go-pg/pg"
)

var g struct {
	visitedDirs map[string]struct{}
	migrations  []Migration
}

type migrateFunc func(db DB, m *Migration, oldVersion int64) (int64, error)

type Migration struct {
	Version       int64
	Transactional bool
	Up            func(DB) error
	Down          func(DB) error
}

func (m *Migration) String() string {
	return strconv.FormatInt(m.Version, 10)
}

// Register registers new database migration. Must be called
// from file with name like "1_initialize_db.go", where:
//   - 1 - migration version;
//   - initialize_db - comment.
func Register(fns ...func(DB) error) error {
	return registerMigration(false, fns...)
}

// RegisterTx is just like Register but marks the migration to be executed inside a transaction.
func RegisterTx(fns ...func(DB) error) error {
	return registerMigration(true, fns...)
}

func registerMigration(isTransactional bool, fns ...func(DB) error) error {
	var up, down func(DB) error
	switch len(fns) {
	case 0:
		return errors.New("Register expects at least 1 arg")
	case 1:
		up = fns[0]
	case 2:
		up = fns[0]
		down = fns[1]
	default:
		return fmt.Errorf("Register expects at most 2 args, got %d", len(fns))
	}

	file := migrationFile()
	version, err := extractVersionGo(file)
	if err != nil {
		return err
	}

	g.migrations = append(g.migrations, Migration{
		Version:       version,
		Transactional: isTransactional,
		Up:            up,
		Down:          down,
	})

	return discoverSQLMigrations(file)
}

func migrationFile() string {
	for i := 2; i < 10; i++ {
		pc, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}

		f := runtime.FuncForPC(pc)
		if f == nil {
			break
		}
		if strings.Contains(f.Name(), "go-pg") {
			continue
		}

		return file
	}
	return ""
}

func discoverSQLMigrations(file string) error {
	dir := filepath.Dir(file)

	if _, ok := g.visitedDirs[dir]; ok {
		return nil
	}
	if g.visitedDirs == nil {
		g.visitedDirs = make(map[string]struct{})
	}
	g.visitedDirs[dir] = struct{}{}

	var ms []Migration
	newMigration := func(version int64) *Migration {
		for i := range ms {
			m := &ms[i]
			if m.Version == version {
				return m
			}
		}

		ms = append(ms, Migration{
			Version: version,
		})
		return &ms[len(ms)-1]
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".sql") {
			return nil
		}

		base := filepath.Base(path)
		idx := strings.IndexByte(base, '_')
		if idx == -1 {
			err := fmt.Errorf(
				"file=%q must have name in format version_comment, e.g. 1_initial",
				base)
			return err
		}

		version, err := strconv.ParseInt(base[:idx], 10, 64)
		if err != nil {
			return err
		}

		m := newMigration(version)
		if strings.HasSuffix(base, ".up.sql") {
			if m.Up != nil {
				return fmt.Errorf("migration=%d already has Up func", version)
			}
			m.Up = newSQLMigration(path)
			return nil
		}
		if strings.HasSuffix(base, ".down.sql") {
			if m.Down != nil {
				return fmt.Errorf("migration=%d already has Down func", version)
			}
			m.Down = newSQLMigration(path)
			return nil
		}

		return fmt.Errorf("file=%q must have extension .up.sql or .down.sql", base)
	})
	if err != nil {
		return err
	}

	g.migrations = append(g.migrations, ms...)
	return nil
}

func newSQLMigration(path string) func(DB) error {
	return func(db DB) error {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = db.Exec(string(b))
		return err
	}
}

func MustRegister(fns ...func(DB) error) {
	err := Register(fns...)
	if err != nil {
		panic(err)
	}
}

func MustRegisterTx(fns ...func(DB) error) {
	err := RegisterTx(fns...)
	if err != nil {
		panic(err)
	}
}

// RegisteredMigrations returns currently registered Migrations.
func RegisteredMigrations() []Migration {
	// Make a copy to avoid side effects.
	migrations := make([]Migration, len(g.migrations))
	copy(migrations, g.migrations)
	return migrations
}

// Run runs command on the db. Supported commands are:
// - up [target] - runs all available migrations by default or up to target one if argument is provided.
// - down - reverts last migration.
// - reset - reverts all migrations.
// - version - prints current db version.
// - set_version - sets db version without running migrations.
func Run(db DB, a ...string) (oldVersion, newVersion int64, err error) {
	return RunMigrations(db, RegisteredMigrations(), a...)
}

// RunMigrations is like Run, but accepts list of migrations.
func RunMigrations(db DB, migrations []Migration, a ...string) (oldVersion, newVersion int64, err error) {
	sortMigrations(migrations)

	err = validateMigrations(migrations)
	if err != nil {
		return
	}

	cmd := "up"
	if len(a) > 0 {
		cmd = a[0]
	}

	err = createTables(db)
	if err != nil {
		return
	}

	oldVersion, err = Version(db)
	if err != nil {
		return
	}
	newVersion = oldVersion

	switch cmd {
	case "create":
		if len(a) < 2 {
			fmt.Println("Please enter migration description")
			return
		}

		var version int64
		if len(migrations) > 0 {
			version = migrations[len(migrations)-1].Version
		}

		filename := fmtMigrationFilename(version+1, strings.Join(a[1:], "_"))
		err = createMigrationFile(filename)
		if err != nil {
			return
		}

		fmt.Println("created migration", filename)
		return
	case "version":
		return
	case "up":
		var target int64 = math.MaxInt64
		if len(a) > 1 {
			target, err = strconv.ParseInt(a[1], 10, 64)
			if err != nil {
				return
			}
			if oldVersion > target {
				err = fmt.Errorf("old version is larger than target")
				return
			}
		}

		for i := range migrations {
			m := &migrations[i]
			if m.Version > target {
				break
			}
			if m.Version <= oldVersion {
				continue
			}
			newVersion, err = runMigrateFunc(runUp, db, m, oldVersion)
		}
		return
	case "down":
		newVersion, err = down(db, migrations, oldVersion)
		return
	case "reset":
		version := oldVersion
		for {
			newVersion, err = down(db, migrations, version)
			if err != nil || newVersion == version {
				return
			}
			version = newVersion
		}
	case "set_version":
		if len(a) < 2 {
			err = fmt.Errorf("set_version requires version as 2nd arg, e.g. set_version 42")
			return
		}

		newVersion, err = strconv.ParseInt(a[1], 10, 64)
		if err != nil {
			return
		}
		err = SetVersion(db, newVersion)
		return
	default:
		err = fmt.Errorf("unsupported command: %q", cmd)
		return
	}
}

func validateMigrations(migrations []Migration) error {
	versions := make(map[int64]struct{})
	for _, migration := range migrations {
		version := migration.Version
		if _, ok := versions[version]; ok {
			return fmt.Errorf("multiple migrations with version=%d", version)
		}
		versions[version] = struct{}{}
	}
	return nil
}

func runMigrateFunc(f migrateFunc, db DB, m *Migration, oldVersion int64) (newVersion int64, err error) {
	if m.Transactional {
		switch cxn := db.(type) {
		case *pg.DB:
			err = cxn.RunInTransaction(func(tx *pg.Tx) error {
				newVersion, err = f(tx, m, oldVersion)
				return err
			})
			return
		case *pg.Tx:
			// Whole command is running is a transaction already so skip running another one
			newVersion, err = f(db, m, oldVersion)
			return
		default:
			return oldVersion, fmt.Errorf("db should be either a pg.DB or pg.Tx instance")
		}
	}
	newVersion, err = f(db, m, oldVersion)
	return
}

// func runUp(db DB, m *Migration) (int64, error) {
func runUp(db DB, m *Migration, oldVersion int64) (int64, error) {
	err := m.Up(db)
	if err != nil {
		return oldVersion, err
	}
	err = SetVersion(db, m.Version)
	if err != nil {
		return oldVersion, err
	}
	return m.Version, nil
}

func runDown(db DB, m *Migration, oldVersion int64) (int64, error) {
	if m.Down != nil {
		err := m.Down(db)
		if err != nil {
			return oldVersion, err
		}
	}
	newVersion := m.Version - 1
	err := SetVersion(db, newVersion)
	if err != nil {
		return oldVersion, err
	}
	return newVersion, nil
}

func down(db DB, migrations []Migration, oldVersion int64) (newVersion int64, err error) {
	if oldVersion == 0 {
		return
	}

	var m *Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		mm := &migrations[i]
		if mm.Version <= oldVersion {
			m = mm
			break
		}
	}

	if m == nil {
		newVersion = oldVersion
		return
	}
	newVersion, err = runMigrateFunc(runDown, db, m, oldVersion)
	return
}

func extractVersionGo(name string) (int64, error) {
	base := filepath.Base(name)
	if !strings.HasSuffix(name, ".go") {
		return 0, fmt.Errorf("file=%q must have extension .go", base)
	}

	idx := strings.IndexByte(base, '_')
	if idx == -1 {
		err := fmt.Errorf(
			"file=%q must have name in format version_comment, e.g. 1_initial",
			base)
		return 0, err
	}

	n, err := strconv.ParseInt(base[:idx], 10, 64)
	if err != nil {
		return 0, err
	}

	return n, nil
}

type migrationSorter []Migration

func (ms migrationSorter) Len() int {
	return len(ms)
}

func (ms migrationSorter) Swap(i, j int) {
	ms[i], ms[j] = ms[j], ms[i]
}

func (ms migrationSorter) Less(i, j int) bool {
	return ms[i].Version < ms[j].Version
}

func sortMigrations(migrations []Migration) {
	ms := migrationSorter(migrations)
	sort.Sort(ms)
}

var migrationNameRE = regexp.MustCompile(`[^a-z0-9]+`)

func fmtMigrationFilename(version int64, descr string) string {
	descr = strings.ToLower(descr)
	descr = migrationNameRE.ReplaceAllString(descr, "_")
	return fmt.Sprintf("%d_%s.go", version, descr)
}

func createMigrationFile(filename string) error {
	basepath, err := os.Getwd()
	if err != nil {
		return err
	}
	filename = path.Join(basepath, filename)

	_, err = os.Stat(filename)
	if !os.IsNotExist(err) {
		return fmt.Errorf("file=%q already exists (%s)", filename, err)
	}

	return ioutil.WriteFile(filename, migrationTemplate, 0644)
}

var migrationTemplate = []byte(`package main

import (
	"github.com/go-pg/migrations"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		_, err := db.Exec("")
		return err
	}, func(db migrations.DB) error {
		_, err := db.Exec("")
		return err
	})
}
`)
