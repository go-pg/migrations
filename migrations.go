package migrations

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var (
	theMigrations []Migration
)

type Migration struct {
	Version int64
	Up      func(DB) error
	Down    func(DB) error
}

func (m *Migration) String() string {
	return strconv.FormatInt(m.Version, 10)
}

// Register registers new database migration. Must be called
// from file with name like "1_initialize_db.go", where:
// - 1 - migration version;
// - initialize_db - comment.
func Register(up, down func(DB) error) error {
	_, file, _, _ := runtime.Caller(1)
	version, err := extractVersion(file)
	if err != nil {
		return err
	}

	theMigrations = append(theMigrations, Migration{
		Version: version,
		Up:      up,
		Down:    down,
	})
	return nil
}

// Run runs command on the db. Supported commands are:
// - init - creates gopg_migrations table.
// - up - runs all available migrations.
// - down - reverts last migration.
// - version - prints current db version.
// - set_version - sets db version without running migrations.
func Run(db DB, a ...string) (oldVersion, newVersion int64, err error) {
	// Make a copy so there are no side effects of sorting.
	migrations := make([]Migration, len(theMigrations))
	copy(migrations, theMigrations)
	return RunMigrations(db, migrations, a...)
}

// RunMigrations is like Run, but accepts list of migrations.
func RunMigrations(db DB, migrations []Migration, a ...string) (oldVersion, newVersion int64, err error) {
	sortMigrations(migrations)

	var cmd string
	if len(a) > 0 {
		cmd = a[0]
	}

	if cmd == "init" {
		err = createTables(db)
		if err != nil {
			return
		}
		cmd = "version"
	}

	oldVersion, err = Version(db)
	if err != nil {
		return
	}
	newVersion = oldVersion

	switch cmd {
	case "version":
		return
	case "up", "":
		for i := range migrations {
			m := &migrations[i]
			if m.Version <= oldVersion {
				continue
			}
			err = m.Up(db)
			if err != nil {
				return
			}
			newVersion = m.Version
			err = SetVersion(db, newVersion)
			if err != nil {
				return
			}
		}
		return
	case "down":
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
			err = fmt.Errorf("migration %d not found\n", oldVersion)
			return
		}

		if m.Down != nil {
			err = m.Down(db)
			if err != nil {
				return
			}
		}

		newVersion = m.Version - 1
		err = SetVersion(db, newVersion)
		if err != nil {
			return
		}
		return
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

func extractVersion(name string) (int64, error) {
	base := filepath.Base(name)

	if ext := filepath.Ext(base); ext != ".go" {
		return 0, fmt.Errorf("can not extract version from %q", base)
	}

	idx := strings.IndexByte(base, '_')
	if idx == -1 {
		return 0, fmt.Errorf("can not extract version from %q", base)
	}

	n, err := strconv.ParseInt(base[:idx], 10, 64)
	if err != nil {
		return 0, err
	}

	if n <= 0 {
		return 0, errors.New("version must be greater than zero")
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
