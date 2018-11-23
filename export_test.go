package migrations

func Set(ms []Migration) {
	DefaultCollection = DefaultCollection.WithRegisteredMigrations(ms)
}
