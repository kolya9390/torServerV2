package settings

import (
	"errors"
	"fmt"

	"server/log"
)

// SwitchSettingsStorage - simplified version.
func SwitchSettingsStorage(useJSON bool) error {
	if IsReadOnlyMode() {
		return errors.New("read-only mode")
	}
	// Acquire exclusive lock for migration
	dbMigrationLock.Lock()
	defer dbMigrationLock.Unlock()
	runtimePath := currentRuntimePath()

	bboltDB := NewTDBAtPath(runtimePath)
	if bboltDB == nil {
		return errors.New("failed to open BBolt DB")
	}
	// DON'T CLOSE! They're still in use by tdb

	jsonDB := NewJSONDBAtPath(runtimePath)
	if jsonDB == nil {
		return errors.New("failed to open JSON DB")
	}
	// DON'T CLOSE! They're still in use by tdb

	log.TLogln("Switching Settings storage to " + map[bool]string{true: "JSON", false: "BBolt"}[useJSON])

	curSets := currentStoredSettings()
	if curSets != nil {
		curSets.StoreSettingsInJSON = useJSON
		SetBTSets(curSets)
	}

	var err error
	if useJSON {
		err = MigrateSettingsToJSON(bboltDB, jsonDB)
	} else {
		err = MigrateSettingsFromJSON(jsonDB, bboltDB)
	}

	if err != nil {
		return err
	}

	log.TLogln("Settings storage switched. Restart required for routing changes.")

	return nil
}

// SwitchViewedStorage - simplified version.
func SwitchViewedStorage(useJSON bool) error {
	if IsReadOnlyMode() {
		return errors.New("read-only mode")
	}
	// Acquire exclusive lock for migration
	dbMigrationLock.Lock()
	defer dbMigrationLock.Unlock()
	runtimePath := currentRuntimePath()

	bboltDB := NewTDBAtPath(runtimePath)
	if bboltDB == nil {
		return errors.New("failed to open BBolt DB")
	}
	// DON'T CLOSE! They're still in use by tdb

	jsonDB := NewJSONDBAtPath(runtimePath)
	if jsonDB == nil {
		return errors.New("failed to open JSON DB")
	}
	// DON'T CLOSE! They're still in use by tdb

	log.TLogln("Switching Viewed storage to " + map[bool]string{true: "JSON", false: "BBolt"}[useJSON])

	var err error
	if useJSON {
		err = MigrateViewedToJSON(bboltDB, jsonDB)
		if err == nil {
			bboltDB.Clear("Viewed")
		}
	} else {
		err = MigrateViewedFromJSON(jsonDB, bboltDB)
		if err == nil {
			jsonDB.Clear("Viewed")
		}
	}

	if err != nil {
		return err
	}

	curSets := currentStoredSettings()
	if curSets != nil {
		curSets.StoreViewedInJSON = useJSON
		SetBTSets(curSets)
	}

	log.TLogln("Viewed storage switched. Restart required for routing changes.")

	return nil
}

// Used in /storage/settings web API.
func GetStoragePreferences() map[string]any {
	dbMigrationLock.RLock()
	defer dbMigrationLock.RUnlock()

	prefs := map[string]any{
		"settings": "json",
		"viewed":   "bbolt",
	}

	curSets := currentStoredSettings()
	if curSets != nil {
		persistence := curSets.PersistenceConfig()
		if persistence.SettingsInJSON {
			prefs["settings"] = "json"
		} else {
			prefs["settings"] = "bbolt"
		}

		if persistence.ViewedInJSON {
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

// Used in /storage/settings web API.
func SetStoragePreferences(prefs map[string]any) error {
	curSets := currentStoredSettings()
	if IsReadOnlyMode() || curSets == nil {
		return errors.New("cannot change storage preferences. Read-only mode")
	}

	if IsDebug() {
		log.TLogln(fmt.Sprintf("SetStoragePreferences received: %v", prefs))
	}

	persistence := curSets.PersistenceConfig()

	if settingsPref, ok := prefs["settings"].(string); ok && settingsPref != "" {
		useJSON := settingsPref == "json"
		if IsDebug() {
			log.TLogln(fmt.Sprintf("Changing settings storage to useJSON=%v (was %v)",
				useJSON, persistence.SettingsInJSON))
		}

		if persistence.SettingsInJSON != useJSON {
			if err := SwitchSettingsStorage(useJSON); err != nil {
				return fmt.Errorf("failed to switch settings storage: %w", err)
			}
		}
	}

	if viewedPref, ok := prefs["viewed"].(string); ok && viewedPref != "" {
		useJSON := viewedPref == "json"
		if IsDebug() {
			log.TLogln(fmt.Sprintf("Changing viewed storage to useJSON=%v (was %v)",
				useJSON, persistence.ViewedInJSON))
		}

		if persistence.ViewedInJSON != useJSON {
			if err := SwitchViewedStorage(useJSON); err != nil {
				return fmt.Errorf("failed to switch viewed storage: %w", err)
			}
		}
	}

	return nil
}
