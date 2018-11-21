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
	"github.com/go-pg/pg/types"
)

type Migration struct {
	Version       int64
	Transactional bool
	Up            func(DB) error
	Down          func(DB) error
}

func (m *Migration) String() string {
	return strconv.FormatInt(m.Version, 10)
}

type Collection struct {
	tableName               string
	sqlAutodiscoverDisabled bool

	visitedDirs map[string]struct{}
	migrations  []Migration
}

func NewCollection(migrations ...Migration) *Collection {
	return &Collection{
		tableName:  "gopg_migrations",
		migrations: migrations,
	}
}

func (c *Collection) SetTableName(tableName string) *Collection {
	c.tableName = tableName
	return c
}

func (c *Collection) DisableSQLAutodiscover(flag bool) *Collection {
	c.sqlAutodiscoverDisabled = flag
	return c
}

// Register registers new database migration. Must be called
// from file with name like "1_initialize_db.go", where:
//   - 1 - migration version;
//   - initialize_db - comment.
func (c *Collection) Register(fns ...func(DB) error) error {
	return c.register(false, fns...)
}

func (c *Collection) RegisterTx(fns ...func(DB) error) error {
	return c.register(true, fns...)
}

func (c *Collection) register(tx bool, fns ...func(DB) error) error {
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

	c.migrations = append(c.migrations, Migration{
		Version:       version,
		Transactional: tx,
		Up:            up,
		Down:          down,
	})

	if !c.sqlAutodiscoverDisabled {
		err = c.discoverSQLMigrations(file)
		if err != nil {
			return err
		}
	}

	return nil
}

func migrationFile() string {
	for i := 2; i < 10; i++ {
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}

		if strings.Contains(file, "/go-pg/") {
			continue
		}

		return file
	}
	return ""
}

func (c *Collection) discoverSQLMigrations(file string) error {
	dir := filepath.Dir(file)

	if _, ok := c.visitedDirs[dir]; ok {
		return nil
	}
	if c.visitedDirs == nil {
		c.visitedDirs = make(map[string]struct{})
	}
	c.visitedDirs[dir] = struct{}{}

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
				"file=%q must have name in format version_comment, e.c. 1_initial",
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

	c.migrations = append(c.migrations, ms...)
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

func (c *Collection) MustRegister(fns ...func(DB) error) {
	err := c.Register(fns...)
	if err != nil {
		panic(err)
	}
}

func (c *Collection) MustRegisterTx(fns ...func(DB) error) {
	err := c.RegisterTx(fns...)
	if err != nil {
		panic(err)
	}
}

func (c *Collection) Migrations() []Migration {
	// Make a copy to avoid side effects.
	migrations := make([]Migration, len(c.migrations))
	copy(migrations, c.migrations)
	return migrations
}

func (c *Collection) Run(db DB, a ...string) (oldVersion, newVersion int64, err error) {
	migrations := c.Migrations()
	sortMigrations(migrations)

	err = validateMigrations(migrations)
	if err != nil {
		return
	}

	cmd := "up"
	if len(a) > 0 {
		cmd = a[0]
	}

	err = c.createTables(db)
	if err != nil {
		return
	}

	oldVersion, err = c.Version(db)
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
			newVersion, err = runMigrateFunc(c.runUp, db, m)
			if err != nil {
				return
			}
		}
		return
	case "down":
		newVersion, err = c.down(db, migrations, oldVersion)
		return
	case "reset":
		version := oldVersion
		for {
			newVersion, err = c.down(db, migrations, version)
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
		err = c.SetVersion(db, newVersion)
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

type migrateFunc func(db DB, m *Migration) (int64, error)

func runMigrateFunc(f migrateFunc, db DB, m *Migration) (newVersion int64, err error) {
	if !m.Transactional {
		return f(db, m)
	}

	switch cxn := db.(type) {
	case *pg.DB:
		err = cxn.RunInTransaction(func(tx *pg.Tx) error {
			newVersion, err = f(tx, m)
			return err
		})
		return newVersion, err
	case *pg.Tx:
		// Whole command is running is a transaction already
		// so skip running another one.
		return f(db, m)
	default:
		return 0, fmt.Errorf("db should be either a pg.DB or pg.Tx instance")
	}
}

func (c *Collection) runUp(db DB, m *Migration) (int64, error) {
	err := m.Up(db)
	if err != nil {
		return 0, err
	}

	err = c.SetVersion(db, m.Version)
	if err != nil {
		return 0, err
	}

	return m.Version, nil
}

func (c *Collection) runDown(db DB, m *Migration) (int64, error) {
	if m.Down != nil {
		err := m.Down(db)
		if err != nil {
			return 0, err
		}
	}

	newVersion := m.Version - 1
	err := c.SetVersion(db, newVersion)
	if err != nil {
		return 0, err
	}

	return newVersion, nil
}

func (c *Collection) down(db DB, migrations []Migration, oldVersion int64) (int64, error) {
	if oldVersion == 0 {
		return 0, nil
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
		return oldVersion, nil
	}
	return runMigrateFunc(c.runDown, db, m)
}

func (c *Collection) getTableName() types.ValueAppender {
	return pg.Q(c.tableName)
}

func (c *Collection) Version(db DB) (int64, error) {
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

func (c *Collection) SetVersion(db DB, version int64) error {
	if err := c.createTables(db); err != nil {
		return err
	}

	_, err := db.Exec(`
		INSERT INTO ? (version, created_at) VALUES (?, now())
	`, c.getTableName(), version)
	return err
}

func (c *Collection) createTables(db DB) error {
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
