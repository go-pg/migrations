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

var migrations []*Migration

type Migration struct {
	Version int64
	Up      func(DB) error
	Down    func(DB) error
}

func (m *Migration) String() string {
	return strconv.FormatInt(m.Version, 10)
}

func Register(up, down func(DB) error) error {
	_, file, _, _ := runtime.Caller(1)
	version, err := extractVersion(file)
	if err != nil {
		return err
	}
	migrations = append(migrations, &Migration{
		Version: version,
		Up:      up,
		Down:    down,
	})
	return nil
}

// Run runs command on the db. Supported commands are:
// - up - runs all available migrations.
// - down - reverts last migration.
// - version - prints current db version.
// - set_version - sets db version without running migrations.
func Run(db DB, a ...string) (oldVersion, newVersion int64, err error) {
	sortMigrations(migrations)

	err = createTables(db)
	if err != nil {
		return
	}

	oldVersion, err = Version(db)
	if err != nil {
		return
	}
	newVersion = oldVersion

	cmd := a[0]
	switch cmd {
	case "version":
		fmt.Printf("version is %d\n", oldVersion)
		return
	case "up", "":
		for _, m := range migrations {
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
			mm := migrations[i]
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

	idx := strings.Index(base, "_")
	if idx < 0 {
		idx = strings.Index(base, ".")
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

type migrationSorter []*Migration

func (ms migrationSorter) Len() int { return len(ms) }

func (ms migrationSorter) Swap(i, j int) { ms[i], ms[j] = ms[j], ms[i] }

func (ms migrationSorter) Less(i, j int) bool { return ms[i].Version < ms[j].Version }

func sortMigrations(migrations []*Migration) {
	ms := migrationSorter(migrations)
	sort.Sort(ms)
}
