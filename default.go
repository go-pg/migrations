package migrations

var DefaultCollection = NewCollection()

func SetTableName(name string) {
	DefaultCollection.SetTableName(name)
}

func Version(db DB) (int64, error) {
	return DefaultCollection.Version(db)
}

func SetVersion(db DB, version int64) error {
	return DefaultCollection.SetVersion(db, version)
}

// Register registers new database migration. Must be called
// from file with name like "1_initialize_db.go", where:
//   - 1 - migration version;
//   - initialize_db - comment.
func Register(fns ...func(DB) error) error {
	return DefaultCollection.Register(fns...)
}

// RegisterTx is just like Register but marks the migration to be executed inside a transaction.
func RegisterTx(fns ...func(DB) error) error {
	return DefaultCollection.RegisterTx(fns...)
}

func MustRegister(fns ...func(DB) error) {
	DefaultCollection.MustRegister(fns...)
}

func MustRegisterTx(fns ...func(DB) error) {
	DefaultCollection.MustRegisterTx(fns...)
}

// RegisteredMigrations returns currently registered Migrations.
func RegisteredMigrations() []*Migration {
	return DefaultCollection.Migrations("")
}

// Run runs command on the db. Supported commands are:
// - up [target] - runs all available migrations by default or up to target one if argument is provided.
// - down - reverts last migration.
// - reset - reverts all migrations.
// - version - prints current db version.
// - set_version - sets db version without running migrations.
func Run(db DB, a ...string) (oldVersion, newVersion int64, err error) {
	return DefaultCollection.Run(db, "", a...)
}

// RunWithCustomPath runs command on the db. Supported commands are using a custom path to locate the migrations:
// - up [target] - runs all available migrations by default or up to target one if argument is provided.
// - down - reverts last migration.
// - reset - reverts all migrations.
// - version - prints current db version.
// - set_version - sets db version without running migrations.
func RunWithCustomPath(db DB, migrationsPath string, a ...string) (oldVersion, newVersion int64, err error) {
	return DefaultCollection.Run(db, migrationsPath, a...)
}
