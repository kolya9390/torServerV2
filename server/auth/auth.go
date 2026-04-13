package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

const (
	// UsersBucket is the BBolt bucket name for user credentials.
	usersBucket = "users"
)

var (
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrUserExists is returned when attempting to create a duplicate user.
	ErrUserExists = errors.New("user already exists")
	// ErrInvalidPassword is returned when password verification fails.
	ErrInvalidPassword = errors.New("invalid password")
	// ErrEmptyPassword is returned when password is empty.
	ErrEmptyPassword = errors.New("password cannot be empty")
	// ErrEmptyUsername is returned when username is empty.
	ErrEmptyUsername = errors.New("username cannot be empty")
)

// User represents a stored user with a hashed password.
type User struct {
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

// Store manages user credentials in BBolt.
type Store struct {
	db *bbolt.DB
}

// NewStore creates a new credential store backed by the given BBolt database.
func NewStore(db *bbolt.DB) *Store {
	return &Store{db: db}
}

// AddUser creates a new user with a bcrypt-hashed password.
// Returns ErrUserExists if the username is already taken.
func (s *Store) AddUser(username, password string) error {
	if username == "" {
		return ErrEmptyUsername
	}

	if password == "" {
		return ErrEmptyPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(usersBucket))
		if err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}

		// Check if user already exists
		if bucket.Get([]byte(username)) != nil {
			return ErrUserExists
		}

		user := User{
			Hash:      string(hash),
			CreatedAt: time.Now().UTC(),
		}

		data, err := json.Marshal(user)
		if err != nil {
			return fmt.Errorf("marshal user: %w", err)
		}

		return bucket.Put([]byte(username), data)
	})
}

// RemoveUser deletes a user from the store.
// Returns ErrUserNotFound if the user does not exist.
func (s *Store) RemoveUser(username string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(usersBucket))
		if bucket == nil {
			return ErrUserNotFound
		}

		if bucket.Get([]byte(username)) == nil {
			return ErrUserNotFound
		}

		return bucket.Delete([]byte(username))
	})
}

// VerifyPassword checks if the provided password matches the stored hash.
// Returns ErrUserNotFound or ErrInvalidPassword on failure.
func (s *Store) VerifyPassword(username, password string) error {
	var hash string

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(usersBucket))
		if bucket == nil {
			return ErrUserNotFound
		}

		data := bucket.Get([]byte(username))
		if data == nil {
			return ErrUserNotFound
		}

		var user User
		if err := json.Unmarshal(data, &user); err != nil {
			return fmt.Errorf("unmarshal user: %w", err)
		}

		hash = user.Hash

		return nil
	})
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidPassword
	}

	return nil
}

// ListUsers returns a list of all usernames and their creation dates.
// Password hashes are NOT included for security.
func (s *Store) ListUsers() (map[string]time.Time, error) {
	users := make(map[string]time.Time)

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(usersBucket))
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(k, v []byte) error {
			var user User
			if err := json.Unmarshal(v, &user); err != nil {
				return fmt.Errorf("unmarshal user %s: %w", string(k), err)
			}

			users[string(k)] = user.CreatedAt

			return nil
		})
	})

	return users, err
}

// UserCount returns the number of users in the store.
func (s *Store) UserCount() (int, error) {
	count := 0

	err := s.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte(usersBucket))
		if bucket == nil {
			return nil
		}

		count = bucket.Stats().KeyN

		return nil
	})

	return count, err
}
