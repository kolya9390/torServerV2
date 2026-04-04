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

// Add a global lock for database operations during migration
var dbMigrationLock sync.RWMutex

func IsDebug() bool {
	btsetsMu.RLock()
	defer btsetsMu.RUnlock()
	if BTsets != nil {
		return BTsets.EnableDebug
	}
	return false
}

var (
	tdb      TorrServerDB
	Path     string
	IP       string
	Port     string
	Ssl      bool
	SslPort  string
	ReadOnly bool
	HttpAuth bool
	SearchWA bool
	PubIPv4  string
	PubIPv6  string
	TorAddr  string
	MaxSize  int64
)

func InitSets(readOnly, searchWA bool) error {
	ReadOnly = readOnly
	SearchWA = searchWA

	bboltDB := NewTDB()
	if bboltDB == nil {
		return fmt.Errorf("error open bboltDB: %s", filepath.Join(Path, "config.db"))
	}

	jsonDB := NewJsonDB()
	if jsonDB == nil {
		return errors.New("error open jsonDB")
	}

	// Optional forced migration (for manual control)
	if migrationMode := os.Getenv("TS_MIGRATION_MODE"); migrationMode != "" {
		log.TLogln(fmt.Sprintf("Executing forced migration: %s", migrationMode))
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
	btsetsMu.RLock()
	curSets := BTsets
	btsetsMu.RUnlock()
	if curSets != nil && (curSets.StoreSettingsInJson != settingsStoragePref || curSets.StoreViewedInJson != viewedStoragePref) {
		curSets.StoreSettingsInJson = settingsStoragePref
		curSets.StoreViewedInJson = viewedStoragePref
		SetBTSets(curSets)
	}

	// Migrate old torrents
	MigrateTorrents()

	logConfiguration(settingsStoragePref, viewedStoragePref)
	return nil
}

func determineStoragePreferences(bboltDB, jsonDB TorrServerDB) (settingsInJson, viewedInJson bool) {
	// Try to load existing settings first
	if existing := loadExistingSettings(bboltDB, jsonDB); existing != nil {
		if IsDebug() {
			log.TLogln(fmt.Sprintf("Found settings: StoreSettingsInJson=%v, StoreViewedInJson=%v",
				existing.StoreSettingsInJson, existing.StoreViewedInJson))
		}
		// Check if these are actually set or just default zero values
		// For now, trust the stored values
		return existing.StoreSettingsInJson, existing.StoreViewedInJson
	}

	// Defaults (if not set by user)
	settingsInJson = true // JSON for settings (easy editable)
	viewedInJson = false  // BBolt for viewed (performance)

	// Environment overrides
	if env := os.Getenv("TS_SETTINGS_STORAGE"); env != "" {
		settingsInJson = (env == "json")
	}
	if env := os.Getenv("TS_VIEWED_STORAGE"); env != "" {
		viewedInJson = (env == "json")
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("Using flags: settingsInJson=%v, viewedInJson=%v",
			settingsInJson, viewedInJson))
	}
	return settingsInJson, viewedInJson
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

// func loadExistingSettingsDebug(bboltDB, jsonDB TorrServerDB) *BTSets {
// 	// Try JSON first
// 	if buf := jsonDB.Get("Settings", "BitTorr"); buf != nil {
// 		log.TLogln(fmt.Sprintf("Found settings in JSON, size: %d bytes", len(buf)))
// 		var sets BTSets
// 		if err := json.Unmarshal(buf, &sets); err == nil {
// 			log.TLogln(fmt.Sprintf("Parsed from JSON: StoreSettingsInJson=%v, StoreViewedInJson=%v",
// 				sets.StoreSettingsInJson, sets.StoreViewedInJson))
// 			return &sets
// 		} else {
// 			log.TLogln(fmt.Sprintf("Failed to parse JSON settings: %v", err))
// 		}
// 	} else {
// 		log.TLogln("No settings found in JSON")
// 	}

// 	// Try BBolt
// 	if buf := bboltDB.Get("Settings", "BitTorr"); buf != nil {
// 		log.TLogln(fmt.Sprintf("Found settings in BBolt, size: %d bytes", len(buf)))
// 		var sets BTSets
// 		if err := json.Unmarshal(buf, &sets); err == nil {
// 			log.TLogln(fmt.Sprintf("Parsed from BBolt: StoreSettingsInJson=%v, StoreViewedInJson=%v",
// 				sets.StoreSettingsInJson, sets.StoreViewedInJson))
// 			return &sets
// 		} else {
// 			log.TLogln(fmt.Sprintf("Failed to parse BBolt settings: %v", err))
// 		}
// 	} else {
// 		log.TLogln("No settings found in BBolt")
// 	}

// 	log.TLogln("No existing storage settings found")
// 	return nil
// }

func applyCleanMigrations(bboltDB, jsonDB TorrServerDB, settingsInJson, viewedInJson bool) {
	// Settings migration
	if settingsInJson {
		safeMigrate(bboltDB, jsonDB, "Settings", "BitTorr", "JSON", true)
	} else {
		safeMigrate(jsonDB, bboltDB, "Settings", "BitTorr", "BBolt", true)
	}

	// Viewed migration
	if viewedInJson {
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

func setupDatabaseRouting(bboltDB, jsonDB TorrServerDB, settingsInJson, viewedInJson bool) {
	dbRouter := NewXPathDBRouter()
	registerRoute := func(db TorrServerDB, route string) {
		if err := dbRouter.RegisterRoute(db, route); err != nil {
			log.TLogln("Database route registration failed:", route, err)
		}
	}

	if settingsInJson {
		registerRoute(jsonDB, "Settings")
	} else {
		registerRoute(bboltDB, "Settings")
	}

	if viewedInJson {
		registerRoute(jsonDB, "Viewed")
	} else {
		registerRoute(bboltDB, "Viewed")
	}

	registerRoute(bboltDB, "Torrents")
	tdb = NewDBReadCache(dbRouter)
}

func logConfiguration(settingsInJson, viewedInJson bool) {
	settingsLoc := "JSON"
	if !settingsInJson {
		settingsLoc = "BBolt"
	}
	viewedLoc := "JSON"
	if !viewedInJson {
		viewedLoc = "BBolt"
	}

	log.TLogln(fmt.Sprintf("Storage: Settings->%s, Viewed->%s, Torrents->BBolt",
		settingsLoc, viewedLoc))
}

// SwitchSettingsStorage - simplified version
func SwitchSettingsStorage(useJson bool) error {
	if ReadOnly {
		return errors.New("read-only mode")
	}
	// Acquire exclusive lock for migration
	dbMigrationLock.Lock()
	defer dbMigrationLock.Unlock()

	bboltDB := NewTDB()
	if bboltDB == nil {
		return errors.New("failed to open BBolt DB")
	}
	// DON'T CLOSE! They're still in use by tdb
	// defer bboltDB.CloseDB()

	jsonDB := NewJsonDB()
	if jsonDB == nil {
		return errors.New("failed to open JSON DB")
	}
	// DON'T CLOSE! They're still in use by tdb
	// defer jsonDB.CloseDB()

	log.TLogln(fmt.Sprintf("Switching Settings storage to %s",
		map[bool]string{true: "JSON", false: "BBolt"}[useJson]))

	// Update storage preference (must be called before migrate as this setting migrate too)
	btsetsMu.RLock()
	curSets := BTsets
	btsetsMu.RUnlock()
	if curSets != nil {
		curSets.StoreSettingsInJson = useJson
		SetBTSets(curSets)
	}

	var err error
	if useJson {
		err = MigrateSettingsToJson(bboltDB, jsonDB)
	} else {
		err = MigrateSettingsFromJson(jsonDB, bboltDB)
	}

	if err != nil {
		return err
	}

	log.TLogln("Settings storage switched. Restart required for routing changes.")
	return nil
}

// SwitchViewedStorage - simplified version
func SwitchViewedStorage(useJson bool) error {
	if ReadOnly {
		return errors.New("read-only mode")
	}
	// Acquire exclusive lock for migration
	dbMigrationLock.Lock()
	defer dbMigrationLock.Unlock()

	bboltDB := NewTDB()
	if bboltDB == nil {
		return errors.New("failed to open BBolt DB")
	}
	// DON'T CLOSE! They're still in use by tdb
	// defer bboltDB.CloseDB()

	jsonDB := NewJsonDB()
	if jsonDB == nil {
		return errors.New("failed to open JSON DB")
	}
	// DON'T CLOSE! They're still in use by tdb
	// defer jsonDB.CloseDB()

	log.TLogln(fmt.Sprintf("Switching Viewed storage to %s",
		map[bool]string{true: "JSON", false: "BBolt"}[useJson]))

	var err error
	if useJson {
		err = MigrateViewedToJson(bboltDB, jsonDB)
		if err == nil {
			bboltDB.Clear("Viewed")
		}
	} else {
		err = MigrateViewedFromJson(jsonDB, bboltDB)
		if err == nil {
			jsonDB.Clear("Viewed")
		}
	}

	if err != nil {
		return err
	}

	// Update preference
	btsetsMu.RLock()
	curSets := BTsets
	btsetsMu.RUnlock()
	if curSets != nil {
		curSets.StoreViewedInJson = useJson
		SetBTSets(curSets)
	}

	log.TLogln("Viewed storage switched. Restart required for routing changes.")
	return nil
}

// Used in /storage/settings web API
func GetStoragePreferences() map[string]interface{} {
	dbMigrationLock.RLock()
	defer dbMigrationLock.RUnlock()

	prefs := map[string]interface{}{
		"settings": "json",  // Default fallback
		"viewed":   "bbolt", // Default fallback
	}

	btsetsMu.RLock()
	curSets := BTsets
	btsetsMu.RUnlock()
	if curSets != nil {
		// Convert boolean preferences to string values
		if curSets.StoreSettingsInJson {
			prefs["settings"] = "json"
		} else {
			prefs["settings"] = "bbolt"
		}

		if curSets.StoreViewedInJson {
			prefs["viewed"] = "json"
		} else {
			prefs["viewed"] = "bbolt"
		}
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("GetStoragePreferences: settings=%s, viewed=%s",
			prefs["settings"], prefs["viewed"]))
	}
	if tdb != nil {
		prefs["viewedCount"] = len(tdb.List("Viewed"))
	}

	return prefs
}

// Used in /storage/settings web API
func SetStoragePreferences(prefs map[string]interface{}) error {
	btsetsMu.RLock()
	curSets := BTsets
	btsetsMu.RUnlock()
	if ReadOnly || curSets == nil {
		return errors.New("cannot change storage preferences. Read-only mode")
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("SetStoragePreferences received: %v", prefs))
	}

	// Apply changes
	if settingsPref, ok := prefs["settings"].(string); ok && settingsPref != "" {
		useJson := (settingsPref == "json")
		if IsDebug() {
			log.TLogln(fmt.Sprintf("Changing settings storage to useJson=%v (was %v)",
				useJson, BTsets.StoreSettingsInJson))
		}
		if curSets.StoreSettingsInJson != useJson {
			if err := SwitchSettingsStorage(useJson); err != nil {
				return fmt.Errorf("failed to switch settings storage: %w", err)
			}
		}
	}

	if viewedPref, ok := prefs["viewed"].(string); ok && viewedPref != "" {
		useJson := (viewedPref == "json")
		if IsDebug() {
			log.TLogln(fmt.Sprintf("Changing viewed storage to useJson=%v (was %v)",
				useJson, BTsets.StoreViewedInJson))
		}
		if curSets.StoreViewedInJson != useJson {
			if err := SwitchViewedStorage(useJson); err != nil {
				return fmt.Errorf("failed to switch viewed storage: %w", err)
			}
		}
	}

	return nil
}

func CloseDB() {
	if tdb != nil {
		tdb.CloseDB()
	}
}
