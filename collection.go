package migrations

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/go-pg/pg/v10"
)

type Migration struct {
	Version int64

	UpTx bool
	Up   func(DB) error

	DownTx bool
	Down   func(DB) error
}

func (m *Migration) String() string {
	return strconv.FormatInt(m.Version, 10)
}

type Collection struct {
	tableName               string
	sqlAutodiscoverDisabled bool

	mu          sync.Mutex
	visitedDirs map[string]struct{}
	migrations  []*Migration // sorted
}

func NewCollection(migrations ...*Migration) *Collection {
	c := &Collection{
		tableName: "gopg_migrations",
	}
	for _, m := range migrations {
		c.addMigration(m)
	}
	return c
}

func (c *Collection) SetTableName(tableName string) *Collection {
	c.tableName = tableName
	return c
}

func (c *Collection) schemaTableName() (string, string) {
	if ind := strings.IndexByte(c.tableName, '.'); ind >= 0 {
		return c.tableName[:ind], c.tableName[ind+1:]
	}
	return "public", c.tableName
}

func (c *Collection) DisableSQLAutodiscover(flag bool) *Collection {
	c.sqlAutodiscoverDisabled = flag
	return c
}

// Register registers new database migration. Must be called
// from a file with name like "1_initialize_db.go".
func (c *Collection) Register(fns ...func(DB) error) error {
	return c.register(false, fns...)
}

// RegisterTx is like Register, but migration will be run in a transaction.
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

	if !c.sqlAutodiscoverDisabled {
		err = c.DiscoverSQLMigrations(filepath.Dir(file))
		if err != nil {
			return err
		}
	}

	c.addMigration(&Migration{
		Version: version,

		UpTx: tx,
		Up:   up,

		DownTx: tx,
		Down:   down,
	})

	return nil
}

func migrationFile() string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(1, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])

	for {
		f, ok := frames.Next()
		if !ok {
			break
		}
		if !strings.Contains(f.Function, "/go-pg/migrations") {
			return f.File
		}
	}

	return ""
}

// DiscoverSQLMigrations scan the dir for files with .sql extension
// and adds discovered SQL migrations to the collection.
func (c *Collection) DiscoverSQLMigrations(dir string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	return c.DiscoverSQLMigrationsFromFilesystem(osfilesystem{}, dir)
}

// DiscoverSQLMigrations scan the dir from the given filesystem for files with .sql extension
// and adds discovered SQL migrations to the collection.
func (c *Collection) DiscoverSQLMigrationsFromFilesystem(fs http.FileSystem, dir string) error {
	if c.isVisitedDir(dir) {
		return nil
	}

	f, err := fs.Open(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Stat(); os.IsNotExist(err) {
		return nil
	}

	var ms []*Migration
	newMigration := func(version int64) *Migration {
		for i := range ms {
			m := ms[i]
			if m.Version == version {
				return m
			}
		}

		ms = append(ms, &Migration{
			Version: version,
		})
		return ms[len(ms)-1]
	}

	files, err := f.Readdir(-1)
	if err != nil {
		return err
	}

	// Sort files to have consistent errors.
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	for _, f := range files {
		if f.IsDir() {
			continue
		}

		fileName := f.Name()
		if !strings.HasSuffix(fileName, ".sql") {
			continue
		}

		idx := strings.IndexByte(fileName, '_')
		if idx == -1 {
			err := fmt.Errorf(
				"file=%q must have name in format version_comment, e.g. 1_initial",
				fileName)
			return err
		}

		version, err := strconv.ParseInt(fileName[:idx], 10, 64)
		if err != nil {
			return err
		}

		m := newMigration(version)
		filePath := filepath.Join(dir, fileName)

		if strings.HasSuffix(fileName, ".up.sql") {
			if m.Up != nil {
				return fmt.Errorf("migration=%d already has Up func", version)
			}
			m.UpTx = strings.HasSuffix(fileName, ".tx.up.sql")
			m.Up = newSQLMigration(fs, filePath)
			continue
		}

		if strings.HasSuffix(fileName, ".down.sql") {
			if m.Down != nil {
				return fmt.Errorf("migration=%d already has Down func", version)
			}
			m.DownTx = strings.HasSuffix(fileName, ".tx.down.sql")
			m.Down = newSQLMigration(fs, filePath)
			continue
		}

		return fmt.Errorf(
			"file=%q must have extension .up.sql or .down.sql", fileName)
	}

	for _, m := range ms {
		c.addMigration(m)
	}

	return nil
}

func (c *Collection) isVisitedDir(dir string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.visitedDirs[dir]; ok {
		return true
	}

	if c.visitedDirs == nil {
		c.visitedDirs = make(map[string]struct{})
	}
	c.visitedDirs[dir] = struct{}{}

	return false
}

func newSQLMigration(fs http.FileSystem, filePath string) func(DB) error {
	return func(db DB) error {
		f, err := fs.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)

		var query []byte
		var queries []string
		for scanner.Scan() {
			b := scanner.Bytes()

			const prefix = "--gopg:"
			if bytes.HasPrefix(b, []byte(prefix)) {
				b = b[len(prefix):]
				if bytes.Equal(b, []byte("split")) {
					queries = append(queries, string(query))
					query = query[:0]
					continue
				}
				return fmt.Errorf("unknown gopg directive: %q", b)
			}

			query = append(query, b...)
			query = append(query, '\n')
		}
		if len(query) > 0 {
			queries = append(queries, string(query))
		}

		if err := scanner.Err(); err != nil {
			return err
		}

		if len(queries) > 1 {
			switch v := db.(type) {
			case *pg.DB:
				conn := v.Conn()
				defer conn.Close()
				db = conn
			}
		}

		for _, q := range queries {
			_, err = db.Exec(q)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

func (c *Collection) addMigration(migration *Migration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, m := range c.migrations {
		if m.Version > migration.Version {
			c.migrations = insert(c.migrations, i, migration)
			return
		}
	}

	c.migrations = append(c.migrations, migration)
}

func insert(s []*Migration, i int, x *Migration) []*Migration {
	s = append(s, nil)
	copy(s[i+1:], s[i:])
	s[i] = x
	return s
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

func (c *Collection) Migrations() []*Migration {
	if !c.sqlAutodiscoverDisabled {
		_ = c.DiscoverSQLMigrations(filepath.Dir(migrationFile()))

		dir, err := os.Getwd()
		if err == nil {
			_ = c.DiscoverSQLMigrations(dir)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Make a copy to avoid side effects.
	migrations := make([]*Migration, len(c.migrations))
	copy(migrations, c.migrations)

	return migrations
}

func (c *Collection) Run(db DB, a ...string) (oldVersion, newVersion int64, err error) {
	migrations := c.Migrations()
	err = validateMigrations(migrations)
	if err != nil {
		return
	}

	cmd := "up"
	if len(a) > 0 {
		cmd = a[0]
	}

	switch cmd {
	case "init":
		err = c.createTable(db)
		if err != nil {
			return
		}
		return
	case "create":
		if len(a) < 2 {
			fmt.Println("please provide migration description")
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

		fmt.Println("created new migration", filename)
		return
	}

	exists, err := c.tableExists(db)
	if err != nil {
		return
	}
	if !exists {
		err = fmt.Errorf("table %q does not exist; did you run init?", c.tableName)
		return
	}

	tx, version, err := c.begin(db)
	if err != nil {
		return
	}
	defer tx.Close() //nolint

	oldVersion = version
	newVersion = version

	switch cmd {
	case "version":
	case "up":
		target := int64(math.MaxInt64)
		if len(a) > 1 {
			target, err = strconv.ParseInt(a[1], 10, 64)
			if err != nil {
				return
			}
			if version > target {
				break
			}
		}

		for _, m := range migrations {
			if m.Version > target {
				break
			}

			if tx == nil {
				tx, version, err = c.begin(db)
				if err != nil {
					return
				}
			}

			if m.Version <= version {
				continue
			}

			newVersion, err = c.runUp(db, tx, m)
			if err != nil {
				return
			}

			err = tx.Commit()
			if err != nil {
				return
			}
			tx = nil
		}
	case "down":
		newVersion, err = c.down(db, tx, migrations, version)
		if err != nil {
			return
		}
	case "reset":
		for {
			if tx == nil {
				tx, version, err = c.begin(db)
				if err != nil {
					return
				}
			}

			newVersion, err = c.down(db, tx, migrations, version)
			if err != nil {
				return
			}

			err = tx.Commit()
			if err != nil {
				return
			}
			tx = nil

			if newVersion == version {
				break
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
		err = c.SetVersion(tx, newVersion)
		if err != nil {
			return
		}
	default:
		err = fmt.Errorf("unsupported command: %q", cmd)
		if err != nil {
			return
		}
	}

	if tx != nil {
		err = tx.Commit()
	}
	return
}

func validateMigrations(migrations []*Migration) error {
	versions := make(map[int64]struct{}, len(migrations))
	for _, m := range migrations {
		if _, ok := versions[m.Version]; ok {
			return fmt.Errorf(
				"there are multiple migrations with version=%d", m.Version)
		}
		versions[m.Version] = struct{}{}
	}
	return nil
}

func (c *Collection) runUp(db DB, tx *pg.Tx, m *Migration) (int64, error) {
	if m.UpTx {
		db = tx
	}
	return c.run(tx, func() (int64, error) {
		err := m.Up(db)
		if err != nil {
			return 0, err
		}
		return m.Version, nil
	})
}

func (c *Collection) runDown(db DB, tx *pg.Tx, m *Migration) (int64, error) {
	if m.DownTx {
		db = tx
	}
	return c.run(tx, func() (int64, error) {
		if m.Down != nil {
			err := m.Down(db)
			if err != nil {
				return 0, err
			}
		}
		return m.Version - 1, nil
	})
}

func (c *Collection) run(
	tx *pg.Tx, fn func() (int64, error),
) (newVersion int64, err error) {
	newVersion, err = fn()
	if err != nil {
		return
	}
	err = c.SetVersion(tx, newVersion)
	return
}

func (c *Collection) down(db DB, tx *pg.Tx, migrations []*Migration, oldVersion int64) (int64, error) {
	if oldVersion == 0 {
		return 0, nil
	}

	var m *Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		mm := migrations[i]
		if mm.Version <= oldVersion {
			m = mm
			break
		}
	}

	if m == nil {
		return oldVersion, nil
	}
	return c.runDown(db, tx, m)
}

func (c *Collection) schemaExists(db DB) (bool, error) {
	schema, _ := c.schemaTableName()
	return db.Model().
		Table("information_schema.schemata").
		Where("schema_name = '?'", pg.SafeQuery(schema)).
		Exists()
}

func (c *Collection) tableExists(db DB) (bool, error) {
	schema, table := c.schemaTableName()
	return db.Model().
		Table("pg_tables").
		Where("schemaname = '?'", pg.SafeQuery(schema)).
		Where("tablename = '?'", pg.SafeQuery(table)).
		Exists()
}

func (c *Collection) Version(db DB) (int64, error) {
	var version int64
	_, err := db.QueryOne(pg.Scan(&version), `
		SELECT version FROM ? ORDER BY id DESC LIMIT 1
	`, pg.SafeQuery(c.tableName))
	if err != nil {
		if err == pg.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return version, nil
}

func (c *Collection) SetVersion(db DB, version int64) error {
	_, err := db.Exec(`
		INSERT INTO ? (version, created_at) VALUES (?, now())
	`, pg.SafeQuery(c.tableName), version)
	return err
}

func (c *Collection) createTable(db DB) error {
	exists, err := c.schemaExists(db)
	if err != nil {
		return err
	}
	if !exists {
		schema, _ := c.schemaTableName()
		_, err := db.Exec(`CREATE SCHEMA IF NOT EXISTS ?`, pg.SafeQuery(schema))
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS ? (
			id serial,
			version bigint,
			created_at timestamptz
		)
	`, pg.SafeQuery(c.tableName))
	return err
}

const (
	cockroachdbErrorMatch = `at or near "lock"`
	yugabytedbErrorMatch  = `lock mode not supported yet`
)

func (c *Collection) begin(db DB) (*pg.Tx, int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, 0, err
	}

	// If there is an error setting this, rollback the transaction and don't bother doing it
	// because Postgres < 9.6 doesn't support this
	_, err = tx.Exec("SET idle_in_transaction_session_timeout = 0")
	if err != nil {
		_ = tx.Rollback()

		tx, err = db.Begin()
		if err != nil {
			return nil, 0, err
		}
	}
	// If there is an error setting this, rollback the transaction and don't bother doing it
	// because neither CockroachDB nor Yugabyte support it
	_, err = tx.Exec("LOCK TABLE ? IN EXCLUSIVE MODE", pg.SafeQuery(c.tableName))
	if err != nil {
		_ = tx.Rollback()

		if !strings.Contains(err.Error(), cockroachdbErrorMatch) && !strings.Contains(err.Error(), yugabytedbErrorMatch) {
			return nil, 0, err
		}
		tx, err = db.Begin()
		if err != nil {
			return nil, 0, err
		}
	}

	version, err := c.Version(tx)
	if err != nil {
		_ = tx.Rollback()
		return nil, 0, err
	}

	return tx, version, nil
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

	return ioutil.WriteFile(filename, migrationTemplate, 0o644)
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

type osfilesystem struct{}

func (osfilesystem) Open(name string) (http.File, error) {
	return os.Open(name)
}
