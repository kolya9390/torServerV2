package settings

import (
	"fmt"

	"server/log"
)

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
