package migrations

func Set(ms []Migration) {
	DefaultGroup.migrations = ms
}
