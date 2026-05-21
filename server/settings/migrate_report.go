package settings

type MigrationEntryReport struct {
	Name   string
	Action string
	Error  string
}

type MigrationReport struct {
	XPath         string
	DryRun        bool
	Total         int
	MigratedCount int
	SkippedCount  int
	FailedCount   int
	Entries       []MigrationEntryReport
}
