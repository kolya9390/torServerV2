package settings

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"server/internal/torrentparse"
	"server/log"

	bolt "go.etcd.io/bbolt"
)

var dbTorrentsName = []byte("Torrents")

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

type torrentBackupDB struct {
	Name      string
	Magnet    string
	InfoBytes []byte
	Hash      string
	Size      int64
	Timestamp int64
	Category  string
	Data      string
}

// loadTorrentFromBucket reads a single torrent from the old BBolt bucket.
// Returns nil if the torrent is incomplete.
func loadTorrentFromBucket(hdb *bolt.Bucket, hash string) *torrentBackupDB {
	torr := &torrentBackupDB{Hash: hash}

	required := map[string]func([]byte){
		"Name": func(v []byte) { torr.Name = string(v) },
		"Link": func(v []byte) { torr.Magnet = string(v) },
		"Size": func(v []byte) { torr.Size = b2i(v) },
	}

	for key, setter := range required {
		val := hdb.Get([]byte(key))
		if val == nil {
			log.TLogln("MigrateTorrents: missing required field", key, "for hash", hash)

			return nil
		}

		setter(val)
	}

	// Optional fields (may not exist in old DB)
	if val := hdb.Get([]byte("Timestamp")); val != nil {
		torr.Timestamp = b2i(val)
	}

	if val := hdb.Get([]byte("Category")); val != nil {
		torr.Category = string(val)
	}

	if val := hdb.Get([]byte("Data")); val != nil {
		torr.Data = string(val)
	}

	return torr
}

// MigrateTorrents migrates torrents from torrserver.db to config.db.
// Also migrates Category and Data fields if they exist in the old DB.
func MigrateTorrent() {
	srcPath := filepath.Join(Path, "torrserver.db")
	if _, err := os.Lstat(srcPath); os.IsNotExist(err) {
		return
	}

	db, err := bolt.Open(srcPath, 0o666, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		log.TLogln("MigrateTorrents: open source db error:", err)

		return
	}

	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.TLogln("MigrateTorrents close source db error:", closeErr)
		}
	}()

	torrs := loadAllTorrents(db)

	if len(torrs) == 0 {
		log.TLogln("MigrateTorrents: no torrents found in old db")

		return
	}

	migrateAndSave(torrs)

	if err := os.Remove(srcPath); err != nil && !os.IsNotExist(err) {
		log.TLogln("MigrateTorrents remove source db error:", err)
	}
}

// loadAllTorrents reads all torrents from the old BBolt database.
func loadAllTorrents(db *bolt.DB) []*torrentBackupDB {
	var torrs []*torrentBackupDB

	err := db.View(func(tx *bolt.Tx) error {
		tdb := tx.Bucket(dbTorrentsName)
		if tdb == nil {
			return nil
		}

		c := tdb.Cursor()

		for h, _ := c.First(); h != nil; h, _ = c.Next() {
			hdb := tdb.Bucket(h)
			if hdb == nil {
				continue
			}

			if torr := loadTorrentFromBucket(hdb, string(h)); torr != nil {
				torrs = append(torrs, torr)
			}
		}

		return nil
	})

	if err != nil {
		log.TLogln("MigrateTorrents: read error:", err)
	}

	return torrs
}

// migrateAndSave converts and saves old-format torrents to new format.
func migrateAndSave(torrs []*torrentBackupDB) {
	saved := 0
	skipped := 0

	for _, oldTorr := range torrs {
		spec, err := torrentparse.ParseLink(oldTorr.Magnet)
		if err != nil {
			log.TLogln("MigrateTorrents: parse link error, skipping", oldTorr.Name)

			skipped++

			continue
		}

		title := oldTorr.Name
		if len(spec.DisplayName) > len(title) {
			title = spec.DisplayName
		}

		log.TLogln("Migrate torrent:", oldTorr.Name, "hash:", oldTorr.Hash, "size:", oldTorr.Size,
			"category:", oldTorr.Category)

		newTorr := &TorrentDB{
			TorrentSpec: spec,
			Title:       title,
			Category:    oldTorr.Category,
			Data:        oldTorr.Data,
			Timestamp:   oldTorr.Timestamp,
			Size:        oldTorr.Size,
		}

		AddTorrent(newTorr)

		saved++
	}

	log.TLogln("MigrateTorrents: saved", saved, "torrents, skipped", skipped)
}

// MigrateSettingsToJSON migrates Settings from BBolt to JSON.
func MigrateSettingsToJSON(bboltDB, jsonDB TorrServerDB) error {
	// if BTsets != nil {
	// 	return errors.New("migration must be called before initializing BTSets")
	// }
	migrated, err := MigrateSingle(bboltDB, jsonDB, "Settings", "BitTorr")
	if migrated {
		log.TLogln("Settings migrated from BBolt to JSON")
	}

	return err
}

// MigrateSettingsFromJSON migrates Settings from JSON to BBolt.
func MigrateSettingsFromJSON(jsonDB, bboltDB TorrServerDB) error {
	// if BTsets != nil {
	// 	return errors.New("migration must be called before initializing BTSets")
	// }
	migrated, err := MigrateSingle(jsonDB, bboltDB, "Settings", "BitTorr")
	if migrated {
		log.TLogln("Settings migrated from JSON to BBolt")
	}

	return err
}

// MigrateViewedToJSON migrates Viewed data from BBolt to JSON.
func MigrateViewedToJSON(bboltDB, jsonDB TorrServerDB) error {
	migrated, skipped, err := MigrateAll(bboltDB, jsonDB, "Viewed")
	log.TLogln(fmt.Sprintf("Viewed->JSON: %d migrated, %d skipped", migrated, skipped))

	return err
}

// MigrateViewedFromJSON migrates Viewed data from JSON to BBolt.
func MigrateViewedFromJSON(jsonDB, bboltDB TorrServerDB) error {
	migrated, skipped, err := MigrateAll(jsonDB, bboltDB, "Viewed")
	log.TLogln(fmt.Sprintf("Viewed->BBolt: %d migrated, %d skipped", migrated, skipped))

	return err
}

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

// SmartMigrate - keep for manual/advanced use.
func SmartMigrate(bboltDB, jsonDB TorrServerDB, forceDirection string) error {
	// if BTsets != nil {
	// 	return errors.New("migration must be called before initializing BTSets")
	// }
	switch forceDirection {
	case "viewed_to_json":
		return MigrateViewedToJSON(bboltDB, jsonDB)
	case "viewed_to_bbolt":
		return MigrateViewedFromJSON(jsonDB, bboltDB)
	case "settings_to_json":
		return MigrateSettingsToJSON(bboltDB, jsonDB)
	case "settings_to_bbolt":
		return MigrateSettingsFromJSON(jsonDB, bboltDB)
	case "sync_both":
		// Simple sync: copy missing data both ways
		if err := migrateMissing(bboltDB, jsonDB, "Settings", "BitTorr"); err != nil {
			return err
		}

		return syncViewedSimple(bboltDB, jsonDB)
	default:
		return fmt.Errorf("unknown migration direction: %s", forceDirection)
	}
}

func isByteArraysEqualJSON(a, b []byte) (bool, error) {
	if len(a) == 0 && len(b) == 0 {
		return true, nil
	}

	if len(a) == 0 || len(b) == 0 {
		return false, nil
	}
	// Quick check: same length and byte equality
	if len(a) == len(b) {
		// Fast path: byte-by-byte comparison
		for i := range a {
			if a[i] != b[i] {
				break // Need to parse as JSON
			}
		}
		// If we get here, bytes are identical
		return true, nil
	}
	// Parse as JSON for structural comparison
	var objectA, objectB any

	if err := json.Unmarshal(a, &objectA); err != nil {
		return false, fmt.Errorf("error unmarshalling A: %w", err)
	}

	if err := json.Unmarshal(b, &objectB); err != nil {
		return false, fmt.Errorf("error unmarshalling B: %w", err)
	}

	return reflect.DeepEqual(objectA, objectB), nil
}

// Optimized version for performance.
func isByteArraysEqualJSONOptimized(a, b []byte) (bool, error) {
	// Fast paths
	if a == nil && b == nil {
		return true, nil
	}

	if len(a) != len(b) {
		return false, nil
	}

	if len(a) == 0 {
		return true, nil
	}
	// Byte equality (fastest check)
	equal := true

	for i := range a {
		if a[i] != b[i] {
			equal = false

			break
		}
	}

	if equal {
		return true, nil
	}
	// Parse as JSON (slower but accurate)
	return isByteArraysEqualJSON(a, b)
}

func verifyMigration(source, target TorrServerDB, xpath, name string, originalData []byte) error {
	// Get migrated data
	migratedData := target.Get(xpath, name)
	if migratedData == nil {
		return fmt.Errorf("migration failed: no data after migration for %s/%s", xpath, name)
	}
	// Compare with original
	if equal, err := isByteArraysEqualJSONOptimized(originalData, migratedData); err != nil {
		return fmt.Errorf("verification failed for %s/%s: %w", xpath, name, err)
	} else if !equal {
		return fmt.Errorf("data mismatch after migration for %s/%s", xpath, name)
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("Verified migration of %s/%s", xpath, name))
	}

	return nil
}

func b2i(v []byte) int64 {
	return int64(binary.BigEndian.Uint64(v))
}

func migrateMissing(db1, db2 TorrServerDB, xpath, name string) error {
	// Copy from db1 to db2 if missing
	if db2.Get(xpath, name) == nil {
		if data := db1.Get(xpath, name); data != nil {
			db2.Set(xpath, name, data)
		}
	}
	// Copy from db2 to db1 if missing
	if db1.Get(xpath, name) == nil {
		if data := db2.Get(xpath, name); data != nil {
			db1.Set(xpath, name, data)
		}
	}

	return nil
}

func syncViewedSimple(bboltDB, jsonDB TorrServerDB) error {
	// Get all hashes from both
	bboltHashes := bboltDB.List("Viewed")
	jsonHashes := jsonDB.List("Viewed")

	allHashes := make(map[string]bool)
	for _, h := range bboltHashes {
		allHashes[h] = true
	}

	for _, h := range jsonHashes {
		allHashes[h] = true
	}

	// For each hash, ensure it exists in both with merged data
	for hash := range allHashes {
		bboltData := bboltDB.Get("Viewed", hash)
		jsonData := jsonDB.Get("Viewed", hash)

		merged := mergeViewedDataSimple(bboltData, jsonData)
		if merged != nil {
			bboltDB.Set("Viewed", hash, merged)
			jsonDB.Set("Viewed", hash, merged)
		}
	}

	return nil
}

func mergeViewedDataSimple(data1, data2 []byte) []byte {
	if data1 == nil && data2 == nil {
		return nil
	}

	if data1 == nil {
		return data2
	}

	if data2 == nil {
		return data1
	}

	// Try to merge
	var indices1, indices2 map[int]struct{}
	if err := json.Unmarshal(data1, &indices1); err != nil {
		log.TLogln("mergeViewedDataSimple unmarshal data1 error:", err)

		return data2
	}

	if err := json.Unmarshal(data2, &indices2); err != nil {
		log.TLogln("mergeViewedDataSimple unmarshal data2 error:", err)

		return data1
	}

	merged := make(map[int]struct{})
	for idx := range indices1 {
		merged[idx] = struct{}{}
	}

	for idx := range indices2 {
		merged[idx] = struct{}{}
	}

	result, err := json.Marshal(merged)
	if err != nil {
		log.TLogln("mergeViewedDataSimple marshal error:", err)

		return data1
	}

	return result
}
