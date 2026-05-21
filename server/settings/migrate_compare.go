package settings

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"

	"server/log"
)

func isByteArraysEqualJSON(first, second []byte) (bool, error) {
	if len(first) == 0 && len(second) == 0 {
		return true, nil
	}

	if len(first) == 0 || len(second) == 0 {
		return false, nil
	}

	// Quick check: same length and byte equality
	if len(first) == len(second) {
		// Fast path: byte-by-byte comparison
		equal := true

		for i := range first {
			if first[i] != second[i] {
				equal = false

				break // Need to parse as JSON
			}
		}

		if equal {
			return true, nil
		}
	}

	// Parse as JSON for structural comparison
	var objectA, objectB any

	if err := json.Unmarshal(first, &objectA); err != nil {
		return false, fmt.Errorf("error unmarshalling A: %w", err)
	}

	if err := json.Unmarshal(second, &objectB); err != nil {
		return false, fmt.Errorf("error unmarshalling B: %w", err)
	}

	return reflect.DeepEqual(objectA, objectB), nil
}

// Optimized version for performance.
func isByteArraysEqualJSONOptimized(first, second []byte) (bool, error) {
	// Fast paths
	if first == nil && second == nil {
		return true, nil
	}

	if len(first) != len(second) {
		return false, nil
	}

	if len(first) == 0 {
		return true, nil
	}
	// Byte equality (fastest check)
	equal := true

	for i := range first {
		if first[i] != second[i] {
			equal = false

			break
		}
	}

	if equal {
		return true, nil
	}
	// Parse as JSON (slower but accurate)
	return isByteArraysEqualJSON(first, second)
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
