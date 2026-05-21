package settings

import (
	"encoding/json"
	"fmt"

	"server/log"
)

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
