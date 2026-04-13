package settings

import (
	"encoding/json"
	"sync"
	"testing"
)

type testDB struct {
	mu      sync.RWMutex
	data    map[string]map[string][]byte
	setHook func(xPath, name string, value []byte) []byte
}

func newTestDB() *testDB {
	return &testDB{
		data: make(map[string]map[string][]byte),
	}
}

func (d *testDB) CloseDB() {}

func (d *testDB) Get(xPath, name string) []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if _, ok := d.data[xPath]; !ok {
		return nil
	}

	value, ok := d.data[xPath][name]
	if !ok {
		return nil
	}

	cp := make([]byte, len(value))
	copy(cp, value)

	return cp
}

func (d *testDB) Set(xPath, name string, value []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.data[xPath]; !ok {
		d.data[xPath] = make(map[string][]byte)
	}

	finalValue := value
	if d.setHook != nil {
		finalValue = d.setHook(xPath, name, value)
	}

	cp := make([]byte, len(finalValue))
	copy(cp, finalValue)
	d.data[xPath][name] = cp
}

func (d *testDB) List(xPath string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	items, ok := d.data[xPath]
	if !ok {
		return nil
	}

	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}

	return names
}

func (d *testDB) Rem(xPath, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.data[xPath]; ok {
		delete(d.data[xPath], name)
	}
}

func (d *testDB) Clear(xPath string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.data, xPath)
}

func (d *testDB) GetRawDB() any { return nil }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	return out
}

func TestMigrateAllDryRunNoWrites(t *testing.T) {
	source := newTestDB()
	target := newTestDB()

	source.Set("Viewed", "one", mustJSON(t, map[string]int{"idx": 1}))
	source.Set("Viewed", "two", mustJSON(t, map[string]int{"idx": 2}))
	target.Set("Viewed", "one", mustJSON(t, map[string]int{"idx": 1}))

	report, err := MigrateAllDryRun(source, target, "Viewed")
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if !report.DryRun {
		t.Fatalf("expected dry-run report")
	}

	if report.MigratedCount != 1 {
		t.Fatalf("expected 1 would-migrate, got %d", report.MigratedCount)
	}

	if report.SkippedCount != 1 {
		t.Fatalf("expected 1 skipped, got %d", report.SkippedCount)
	}

	if got := target.Get("Viewed", "two"); got != nil {
		t.Fatalf("dry run must not write target, got: %s", string(got))
	}
}

func TestMigrateAllWithReportVerificationFailure(t *testing.T) {
	source := newTestDB()
	target := newTestDB()
	target.setHook = func(xPath, name string, value []byte) []byte {
		return mustJSON(t, map[string]string{"corrupted": "true"})
	}

	source.Set("Viewed", "bad", mustJSON(t, map[string]int{"idx": 99}))

	report, err := MigrateAllWithReport(source, target, "Viewed", false)
	if err == nil {
		t.Fatalf("expected verification error")
	}

	if report.FailedCount != 1 {
		t.Fatalf("expected 1 failed, got %d", report.FailedCount)
	}

	if report.MigratedCount != 0 {
		t.Fatalf("expected 0 migrated, got %d", report.MigratedCount)
	}
}

func TestMigrateAllWithRealBackendsAndDryRun(t *testing.T) {
	tmp := t.TempDir()
	Path = tmp

	globalBboltDBMu.Lock()
	globalBboltDB = nil
	globalBboltDBMu.Unlock()
	globalJSONDBMu.Lock()
	globalJSONDB = nil
	globalJSONDBMu.Unlock()

	bboltDB := NewTDB()
	if bboltDB == nil {
		t.Fatalf("failed to init bbolt db")
	}

	jsonDB := NewJSONDB()
	if jsonDB == nil {
		t.Fatalf("failed to init json db")
	}

	t.Cleanup(func() {
		bboltDB.CloseDB()
		globalBboltDBMu.Lock()
		globalBboltDB = nil
		globalBboltDBMu.Unlock()

		globalJSONDBMu.Lock()
		globalJSONDB = nil
		globalJSONDBMu.Unlock()
	})

	bboltDB.Set("Viewed", "hash-a", mustJSON(t, map[string]any{"0": struct{}{}}))
	bboltDB.Set("Viewed", "hash-b", mustJSON(t, map[string]any{"10": struct{}{}}))

	pre, err := MigrateAllDryRun(bboltDB, jsonDB, "Viewed")
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if pre.MigratedCount != 2 || pre.SkippedCount != 0 {
		t.Fatalf("unexpected dry-run report: %+v", pre)
	}

	if len(jsonDB.List("Viewed")) != 0 {
		t.Fatalf("dry run should not write to target")
	}

	post, err := MigrateAllWithReport(bboltDB, jsonDB, "Viewed", false)
	if err != nil {
		t.Fatalf("real migration failed: %v", err)
	}

	if post.MigratedCount != 2 || post.FailedCount != 0 {
		t.Fatalf("unexpected migration report: %+v", post)
	}

	if len(jsonDB.List("Viewed")) != 2 {
		t.Fatalf("expected migrated fixtures in json db")
	}
}
