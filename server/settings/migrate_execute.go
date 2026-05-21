package settings

import (
	"fmt"

	"server/log"
)

// MigrateSingle migrates a single entry with validation
// Returns: (migrated bool, error).
func MigrateSingle(source, target TorrServerDB, xpath, name string) (bool, error) {
	report, err := migrateSingleWithReport(source, target, xpath, name, false)
	if err != nil {
		return false, err
	}

	return report.Action == "migrated", nil
}

func MigrateSingleDryRun(source, target TorrServerDB, xpath, name string) (MigrationEntryReport, error) {
	return migrateSingleWithReport(source, target, xpath, name, true)
}

func migrateSingleWithReport(source, target TorrServerDB, xpath, name string, dryRun bool) (MigrationEntryReport, error) {
	report := MigrationEntryReport{
		Name:   name,
		Action: "skipped",
	}

	sourceData := source.Get(xpath, name)
	if sourceData == nil {
		if IsDebug() {
			log.TLogln(fmt.Sprintf("No data to migrate for %s/%s", xpath, name))
		}

		return report, nil
	}

	targetData := target.Get(xpath, name)
	if targetData != nil {
		// Check if already identical
		if equal, err := isByteArraysEqualJSON(sourceData, targetData); err == nil && equal {
			if IsDebug() {
				log.TLogln(fmt.Sprintf("Skipping %s/%s (already identical)", xpath, name))
			}

			return report, nil
		}
	}

	if dryRun {
		report.Action = "would_migrate"

		return report, nil
	}

	// Perform migration
	target.Set(xpath, name, sourceData)

	if IsDebug() {
		log.TLogln(fmt.Sprintf("Migrating %s/%s", xpath, name))
	}

	// Verify migration
	if err := verifyMigration(source, target, xpath, name, sourceData); err != nil {
		report.Action = "failed"
		report.Error = err.Error()

		return report, fmt.Errorf("migration verification failed for %s/%s: %w", xpath, name, err)
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("Successfully migrated %s/%s", xpath, name))
	}

	report.Action = "migrated"

	return report, nil
}

// MigrateAll migrates all entries in an xpath with validation
// Returns: (migratedCount, skippedCount, error).
func MigrateAll(source, target TorrServerDB, xpath string) (int, int, error) {
	report, err := MigrateAllWithReport(source, target, xpath, false)

	return report.MigratedCount, report.SkippedCount, err
}

func MigrateAllDryRun(source, target TorrServerDB, xpath string) (MigrationReport, error) {
	return MigrateAllWithReport(source, target, xpath, true)
}

func MigrateAllWithReport(source, target TorrServerDB, xpath string, dryRun bool) (MigrationReport, error) {
	report := MigrationReport{
		XPath:  xpath,
		DryRun: dryRun,
	}

	names := source.List(xpath)
	if len(names) == 0 {
		if IsDebug() {
			log.TLogln("No entries to migrate for " + xpath)
		}

		return report, nil
	}

	var firstError error

	report.Total = len(names)

	if IsDebug() {
		log.TLogln(fmt.Sprintf("Starting migration of %d %s entries", len(names), xpath))
	}

	for _, name := range names {
		entryReport, err := migrateSingleWithReport(source, target, xpath, name, dryRun)
		report.Entries = append(report.Entries, entryReport)

		switch entryReport.Action {
		case "migrated":
			report.MigratedCount++
		case "would_migrate":
			report.MigratedCount++
		case "failed":
			report.FailedCount++
		default:
			report.SkippedCount++
		}

		if err != nil && firstError == nil {
			firstError = err
		}

		if err != nil {
			log.TLogln(fmt.Sprintf("Migration failed for %s/%s: %v", xpath, name, err))
		}
	}

	summary := fmt.Sprintf("%s migration complete: %d migrated, %d skipped, %d failed",
		xpath, report.MigratedCount, report.SkippedCount, report.FailedCount)
	if report.DryRun {
		summary = fmt.Sprintf("%s dry-run complete: %d would migrate, %d skipped, %d failed",
			xpath, report.MigratedCount, report.SkippedCount, report.FailedCount)
	}

	if firstError != nil {
		summary += fmt.Sprintf(", 1+ errors (first: %v)", firstError)
	}

	if IsDebug() {
		log.TLogln(summary)
	}

	return report, firstError
}
