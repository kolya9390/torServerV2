package auth

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.etcd.io/bbolt"

	"server/log"
)

// MigrateFromAccsDB migrates users from the legacy accs.db (plain text passwords)
// to the new BBolt-based store with bcrypt-hashed passwords.
// After successful migration, the old accs.db file is deleted.
func MigrateFromAccsDB(store *Store, configPath string) error {
	accsPath := filepath.Join(configPath, "accs.db")

	// Check if legacy file exists
	if _, err := os.Stat(accsPath); os.IsNotExist(err) {
		return nil
	}

	// Check if BBolt already has users (already migrated)
	count, err := store.UserCount()
	if err != nil {
		return fmt.Errorf("check user count: %w", err)
	}

	if count > 0 {
		log.TLogln("Auth migration skipped: users already exist in BBolt")

		return nil
	}

	// Read legacy file
	buf, err := os.ReadFile(accsPath)
	if err != nil {
		return fmt.Errorf("read accs.db: %w", err)
	}

	// Parse plain text passwords
	var accounts map[string]string
	if err := json.Unmarshal(buf, &accounts); err != nil {
		log.TLogln("Error parsing accs.db:", err)

		return nil
	}

	if len(accounts) == 0 {
		log.TLogln("Auth migration: accs.db is empty")

		return nil
	}

	log.TLogln("Auth migration: migrating", len(accounts), "users from accs.db")

	// Add users with bcrypt hashing
	migrated := 0
	failed := 0

	for username, password := range accounts {
		if err := store.AddUser(username, password); err != nil {
			if err == ErrUserExists {
				continue
			}

			log.TLogln("Auth migration: failed to migrate user", username, ":", err)

			failed++

			continue
		}

		migrated++
	}

	// Delete legacy file
	if err := os.Remove(accsPath); err != nil {
		log.TLogln("Auth migration warning: failed to remove accs.db:", err)
	} else {
		log.TLogln("Auth migration: removed legacy accs.db")
	}

	log.TLogln("Auth migration complete:", migrated, "migrated,", failed, "failed")

	return nil
}

// EnsureDefaultAdmin creates an admin user if no users exist in the store.
// This is useful for first-time setup.
func EnsureDefaultAdmin(store *Store, password string) error {
	count, err := store.UserCount()
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	if password == "" {
		return nil
	}

	if err := store.AddUser("admin", password); err != nil {
		return fmt.Errorf("create default admin: %w", err)
	}

	log.TLogln("Created default admin user")

	return nil
}

// TokenStore manages shutdown tokens in BBolt.
type TokenStore struct {
	db *bbolt.DB
}

const (
	secretsBucket = "secrets"
	shutdownKey   = "shutdown_token"
)

// NewTokenStore creates a new token store.
func NewTokenStore(db *bbolt.DB) *TokenStore {
	return &TokenStore{db: db}
}

// GetShutdownToken returns the stored shutdown token.
// Returns empty string if no token is set.
func (ts *TokenStore) GetShutdownToken() (string, error) {
	var token string

	err := ts.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(secretsBucket))
		if bucket == nil {
			return nil
		}

		data := bucket.Get([]byte(shutdownKey))
		if data != nil {
			token = string(data)
		}

		return nil
	})

	return token, err
}

// SetShutdownToken stores a new shutdown token.
func (ts *TokenStore) SetShutdownToken(token string) error {
	return ts.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(secretsBucket))
		if err != nil {
			return err
		}

		return bucket.Put([]byte(shutdownKey), []byte(token))
	})
}

// GenerateAndStoreToken generates a cryptographically secure random token
// and stores it as the shutdown token.
func (ts *TokenStore) GenerateAndStoreToken() (string, error) {
	token, err := GenerateSecureToken(32)
	if err != nil {
		return "", err
	}

	if err := ts.SetShutdownToken(token); err != nil {
		return "", err
	}

	return token, nil
}

// GenerateSecureToken generates a random hex-encoded token of the given byte size.
func GenerateSecureToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := readCryptoRandom(bytes); err != nil {
		return "", err
	}

	return hex.EncodeToString(bytes), nil
}

// EnsureDefaultToken generates a token if none exists and logs it once.
func (ts *TokenStore) EnsureDefaultToken() error {
	token, err := ts.GetShutdownToken()
	if err != nil {
		return err
	}

	if token != "" {
		return nil
	}

	// Generate and store token
	token, err = ts.GenerateAndStoreToken()
	if err != nil {
		return fmt.Errorf("generate shutdown token: %w", err)
	}

	log.TLogln("Generated shutdown token:", token)
	log.TLogln("Store this token securely — it will not be shown again")

	return nil
}

// readCryptoRandom fills the given slice with cryptographically secure random bytes.
func readCryptoRandom(b []byte) (int, error) {
	return readRandom(b)
}
