package auth

import (
	"os"
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return NewStore(db)
}

func TestStore_AddUser(t *testing.T) {
	s := newTestStore(t)

	// Success
	if err := s.AddUser("alice", "supersecret123"); err != nil {
		t.Fatalf("AddUser() error = %v", err)
	}

	// Duplicate
	if err := s.AddUser("alice", "otherpass"); err != ErrUserExists {
		t.Fatalf("AddUser() duplicate error = %v, want %v", err, ErrUserExists)
	}

	// Empty username
	if err := s.AddUser("", "pass"); err != ErrEmptyUsername {
		t.Fatalf("AddUser() empty username = %v, want %v", err, ErrEmptyUsername)
	}

	// Empty password
	if err := s.AddUser("bob", ""); err != ErrEmptyPassword {
		t.Fatalf("AddUser() empty password = %v, want %v", err, ErrEmptyPassword)
	}
}

func TestStore_VerifyPassword(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddUser("alice", "supersecret123"); err != nil {
		t.Fatal(err)
	}

	// Correct password
	if err := s.VerifyPassword("alice", "supersecret123"); err != nil {
		t.Fatalf("VerifyPassword() correct = %v", err)
	}

	// Wrong password
	if err := s.VerifyPassword("alice", "wrongpass"); err == nil {
		t.Fatal("VerifyPassword() wrong = nil, want error")
	}

	// Non-existent user
	if err := s.VerifyPassword("nobody", "pass"); err != ErrUserNotFound {
		t.Fatalf("VerifyPassword() nonexistent = %v, want %v", err, ErrUserNotFound)
	}
}

func TestStore_RemoveUser(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddUser("alice", "supersecret123"); err != nil {
		t.Fatal(err)
	}

	// Success
	if err := s.RemoveUser("alice"); err != nil {
		t.Fatalf("RemoveUser() error = %v", err)
	}

	// Not found
	if err := s.RemoveUser("alice"); err != ErrUserNotFound {
		t.Fatalf("RemoveUser() not found = %v, want %v", err, ErrUserNotFound)
	}
}

func TestStore_ListUsers(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddUser("alice", "pass1"); err != nil {
		t.Fatal(err)
	}

	if err := s.AddUser("bob", "pass2"); err != nil {
		t.Fatal(err)
	}

	users, err := s.ListUsers()
	if err != nil {
		t.Fatal(err)
	}

	if len(users) != 2 {
		t.Fatalf("ListUsers() count = %d, want 2", len(users))
	}

	if _, ok := users["alice"]; !ok {
		t.Error("ListUsers() missing alice")
	}

	if _, ok := users["bob"]; !ok {
		t.Error("ListUsers() missing bob")
	}
}

func TestStore_UserCount(t *testing.T) {
	s := newTestStore(t)

	count, err := s.UserCount()
	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("UserCount() empty = %d, want 0", count)
	}

	if err := s.AddUser("alice", "pass"); err != nil {
		t.Fatal(err)
	}

	count, err = s.UserCount()
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf("UserCount() after add = %d, want 1", count)
	}
}

func TestMigrateFromAccsDB(t *testing.T) {
	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := NewStore(db)

	// Create legacy accs.db
	accsPath := filepath.Join(dir, "accs.db")
	accsContent := `{"admin":"adminpass123","user":"userpass123"}`
	if err := os.WriteFile(accsPath, []byte(accsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run migration
	if err := MigrateFromAccsDB(store, dir); err != nil {
		t.Fatal(err)
	}

	// Verify migration
	count, _ := store.UserCount()
	if count != 2 {
		t.Fatalf("migration: count = %d, want 2", count)
	}

	// Verify passwords work
	if err := store.VerifyPassword("admin", "adminpass123"); err != nil {
		t.Fatalf("migration: admin password check = %v", err)
	}

	// Verify accs.db was removed
	if _, err := os.Stat(accsPath); !os.IsNotExist(err) {
		t.Error("migration: accs.db was not removed")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	t1, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}

	if len(t1) != 64 { // 32 bytes = 64 hex chars
		t.Fatalf("token length = %d, want 64", len(t1))
	}

	t2, _ := GenerateSecureToken(32)
	if t1 == t2 {
		t.Fatal("tokens are identical, expected randomness")
	}
}

func TestTokenStore(t *testing.T) {
	dir := t.TempDir()
	db, err := bbolt.Open(filepath.Join(dir, "test.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ts := NewTokenStore(db)

	// Empty token
	token, err := ts.GetShutdownToken()
	if err != nil {
		t.Fatal(err)
	}

	if token != "" {
		t.Fatalf("empty token = %s, want empty", token)
	}

	// Set token
	if err := ts.SetShutdownToken("mysecret"); err != nil {
		t.Fatal(err)
	}

	token, err = ts.GetShutdownToken()
	if err != nil {
		t.Fatal(err)
	}

	if token != "mysecret" {
		t.Fatalf("token = %s, want mysecret", token)
	}

	// Generate
	genToken, err := ts.GenerateAndStoreToken()
	if err != nil {
		t.Fatal(err)
	}

	if len(genToken) != 64 {
		t.Fatalf("generated token length = %d, want 64", len(genToken))
	}

	// Verify stored
	stored, _ := ts.GetShutdownToken()
	if stored != genToken {
		t.Fatal("stored token doesn't match generated")
	}
}
