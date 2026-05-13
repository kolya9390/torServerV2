package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"server/log"
)

// Add a global lock for database operations during migration.
var dbMigrationLock sync.RWMutex

func IsDebug() bool {
	if sets := currentStoredSettings(); sets != nil {
		return sets.EnableDebug
	}

	return false
}

var (
	tdb TorrServerDB
)

func InitSets(readOnly, searchWA bool) error {
	SetReadOnly(readOnly)
	UpdateRuntimeState(func(runtime *RuntimeState) {
		runtime.SearchWA = searchWA
	})
	runtimePath := currentRuntimePath()

	bboltDB := NewTDBAtPath(runtimePath)
	if bboltDB == nil {
		return fmt.Errorf("error open bboltDB: %s", filepath.Join(runtimePath, "config.db"))
	}

	jsonDB := NewJSONDBAtPath(runtimePath)
	if jsonDB == nil {
		return errors.New("error open jsonDB")
	}

	// Optional forced migration (for manual control)
	if migrationMode := os.Getenv("TS_MIGRATION_MODE"); migrationMode != "" {
		log.TLogln("Executing forced migration: " + migrationMode)

		if err := SmartMigrate(bboltDB, jsonDB, migrationMode); err != nil {
			log.TLogln("Migration warning:", err)
		}
	}

	// Determine storage preferences
	settingsStoragePref, viewedStoragePref := determineStoragePreferences(bboltDB, jsonDB)

	// Apply migrations (clean, one-way)
	applyCleanMigrations(bboltDB, jsonDB, settingsStoragePref, viewedStoragePref)

	// Setup routing
	setupDatabaseRouting(bboltDB, jsonDB, settingsStoragePref, viewedStoragePref)

	// Load settings
	loadBTSets()

	// Update preferences if they changed
	curSets := currentStoredSettings()

	if curSets != nil && (curSets.StoreSettingsInJSON != settingsStoragePref || curSets.StoreViewedInJSON != viewedStoragePref) {
		curSets.StoreSettingsInJSON = settingsStoragePref
		curSets.StoreViewedInJSON = viewedStoragePref
		SetBTSets(curSets)
	}

	// Migrate old torrents
	MigrateTorrentAtPath(runtimePath)

	logConfiguration(settingsStoragePref, viewedStoragePref)

	return nil
}

func determineStoragePreferences(bboltDB, jsonDB TorrServerDB) (settingsInJSON, viewedInJSON bool) {
	// Try to load existing settings first
	if existing := loadExistingSettings(bboltDB, jsonDB); existing != nil {
		if IsDebug() {
			log.TLogln(fmt.Sprintf("Found settings: StoreSettingsInJSON=%v, StoreViewedInJSON=%v",
				existing.StoreSettingsInJSON, existing.StoreViewedInJSON))
		}
		// Check if these are actually set or just default zero values
		// For now, trust the stored values
		return existing.StoreSettingsInJSON, existing.StoreViewedInJSON
	}

	// Defaults (if not set by user)
	settingsInJSON = true // JSON for settings (easy editable)
	viewedInJSON = false  // BBolt for viewed (performance)

	// Environment overrides
	if env := os.Getenv("TS_SETTINGS_STORAGE"); env != "" {
		settingsInJSON = (env == "json")
	}

	if env := os.Getenv("TS_VIEWED_STORAGE"); env != "" {
		viewedInJSON = (env == "json")
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("Using flags: settingsInJSON=%v, viewedInJSON=%v",
			settingsInJSON, viewedInJSON))
	}

	return settingsInJSON, viewedInJSON
}

func loadExistingSettings(bboltDB, jsonDB TorrServerDB) *BTSets {
	// Try JSON first
	if buf := jsonDB.Get("Settings", "BitTorr"); buf != nil {
		var sets BTSets
		if err := json.Unmarshal(buf, &sets); err == nil {
			return &sets
		}
	}
	// Try BBolt
	if buf := bboltDB.Get("Settings", "BitTorr"); buf != nil {
		var sets BTSets
		if err := json.Unmarshal(buf, &sets); err == nil {
			return &sets
		}
	}

	return nil
}

func applyCleanMigrations(bboltDB, jsonDB TorrServerDB, settingsInJSON, viewedInJSON bool) {
	// Settings migration
	if settingsInJSON {
		safeMigrate(bboltDB, jsonDB, "Settings", "BitTorr", "JSON", true)
	} else {
		safeMigrate(jsonDB, bboltDB, "Settings", "BitTorr", "BBolt", true)
	}

	// Viewed migration
	if viewedInJSON {
		safeMigrateAll(bboltDB, jsonDB, "Viewed", "JSON", true)
	} else {
		safeMigrateAll(jsonDB, bboltDB, "Viewed", "BBolt", true)
	}
}

func safeMigrate(source, target TorrServerDB, xpath, name, targetName string, clearSource bool) {
	if IsDebug() {
		log.TLogln(fmt.Sprintf("Checking migration of %s/%s to %s", xpath, name, targetName))
	}

	if dryRunReport, err := MigrateSingleDryRun(source, target, xpath, name); err == nil {
		log.TLogln(fmt.Sprintf("Migration pre-check %s/%s -> %s: %s",
			xpath, name, targetName, dryRunReport.Action))
	}

	migrated, err := MigrateSingle(source, target, xpath, name)
	if err != nil {
		log.TLogln(fmt.Sprintf("Migration error for %s/%s: %v", xpath, name, err))

		return
	}

	if migrated {
		log.TLogln(fmt.Sprintf("Successfully migrated %s/%s to %s", xpath, name, targetName))
		// Clear source if requested
		if clearSource {
			source.Rem(xpath, name)

			if IsDebug() {
				log.TLogln(fmt.Sprintf("Cleared %s/%s from source", xpath, name))
			}
		}
	} else {
		log.TLogln(fmt.Sprintf("No migration needed for %s/%s (already exists or no data)",
			xpath, name))
	}
}

func safeMigrateAll(source, target TorrServerDB, xpath, targetName string, clearSource bool) {
	if IsDebug() {
		log.TLogln(fmt.Sprintf("Starting migration of all %s entries to %s", xpath, targetName))
	}

	if preReport, err := MigrateAllDryRun(source, target, xpath); err == nil {
		log.TLogln(fmt.Sprintf("%s migration pre-check -> %s: %d would migrate, %d skipped, %d failed",
			xpath, targetName, preReport.MigratedCount, preReport.SkippedCount, preReport.FailedCount))
	}

	postReport, err := MigrateAllWithReport(source, target, xpath, false)
	migrated := postReport.MigratedCount
	log.TLogln(fmt.Sprintf("%s migration result -> %s: %d migrated, %d skipped, %d failed",
		xpath, targetName, postReport.MigratedCount, postReport.SkippedCount, postReport.FailedCount))

	if err != nil {
		log.TLogln(fmt.Sprintf("Migration had errors: %v", err))
	}
	// Clear source if requested and we successfully migrated entries
	if clearSource && migrated > 0 {
		sourceCount := len(source.List(xpath))
		// Only clear if we migrated at least as many as were in source
		// (accounting for possible duplicates)
		if migrated >= sourceCount {
			source.Clear(xpath)

			if IsDebug() {
				log.TLogln(fmt.Sprintf("Cleared all %s entries from source", xpath))
			}
		} else {
			log.TLogln(fmt.Sprintf("Not clearing %s: only migrated %d of %d entries",
				xpath, migrated, sourceCount))
		}
	}
}

func setupDatabaseRouting(bboltDB, jsonDB TorrServerDB, settingsInJSON, viewedInJSON bool) {
	dbRouter := NewXPathDBRouter()
	registerRoute := func(db TorrServerDB, route string) {
		if err := dbRouter.RegisterRoute(db, route); err != nil {
			log.TLogln("Database route registration failed:", route, err)
		}
	}

	if settingsInJSON {
		registerRoute(jsonDB, "Settings")
	} else {
		registerRoute(bboltDB, "Settings")
	}

	if viewedInJSON {
		registerRoute(jsonDB, "Viewed")
	} else {
		registerRoute(bboltDB, "Viewed")
	}

	registerRoute(bboltDB, "Torrents")

	tdb = NewDBReadCache(dbRouter)
}

func logConfiguration(settingsInJSON, viewedInJSON bool) {
	settingsLoc := "JSON"
	if !settingsInJSON {
		settingsLoc = "BBolt"
	}

	viewedLoc := "JSON"
	if !viewedInJSON {
		viewedLoc = "BBolt"
	}

	log.TLogln(fmt.Sprintf("Storage: Settings->%s, Viewed->%s, Torrents->BBolt",
		settingsLoc, viewedLoc))
}

func CloseDB() {
	if tdb != nil {
		tdb.CloseDB()
	}
}
