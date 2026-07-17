package releasebundle

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testVersion = "1.0.0-beta.7"

func TestCreateAllProducesReproducibleVerifiedBundles(t *testing.T) {
	t.Parallel()

	repositoryRoot := newTestRepository(t, false)
	firstRelease := newTestRelease(t, testVersion)
	secondRelease := newTestRelease(t, testVersion)

	if err := CreateAll(repositoryRoot, firstRelease, testVersion); err != nil {
		t.Fatalf("create first release: %v", err)
	}

	if err := CreateAll(repositoryRoot, secondRelease, testVersion); err != nil {
		t.Fatalf("create second release: %v", err)
	}

	inspected := make(map[string]int)
	inspect := func(_ string, product, version, commit string) error {
		if version != testVersion || commit != "test-commit" {
			return errors.New("unexpected release metadata")
		}

		inspected[product]++

		return nil
	}

	if err := verifyAll(firstRelease, testVersion, "test-commit", inspect); err != nil {
		t.Fatalf("verify release: %v", err)
	}

	if inspected["torrserver"] != len(releaseTargets) || inspected["torrctl"] != len(releaseTargets) {
		t.Fatalf("inspected binaries = %+v", inspected)
	}

	for _, item := range releaseTargets {
		name := item.bundleName(testVersion)
		first := readTestFile(t, filepath.Join(firstRelease, name))
		second := readTestFile(t, filepath.Join(secondRelease, name))
		if !bytes.Equal(first, second) {
			t.Fatalf("bundle %s is not reproducible", name)
		}

		members, err := readArchive(filepath.Join(firstRelease, name), item.zip)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		names := make([]string, 0, len(members))
		for _, member := range members {
			names = append(names, member.name)
		}

		if !sort.StringsAreSorted(names) {
			t.Fatalf("bundle %s members are not sorted: %v", name, names)
		}
	}

	firstManifest := readTestFile(t, filepath.Join(firstRelease, "torrserver-"+testVersion+"-SHA256SUMS"))
	secondManifest := readTestFile(t, filepath.Join(secondRelease, "torrserver-"+testVersion+"-SHA256SUMS"))
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Fatal("checksum manifest is not reproducible")
	}

	if lines := strings.Count(strings.TrimSpace(string(firstManifest)), "\n") + 1; lines != 15 {
		t.Fatalf("checksum entries = %d, want 15", lines)
	}
}

func TestVerifyBundleRejectsUnsafeOrInvalidMembers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		zipped     bool
		mutate     func(target, []archiveEntry) []archiveEntry
		wantSubstr string
	}{
		{name: "tar missing", mutate: removeConfigEntry, wantSubstr: "missing archive member"},
		{name: "zip missing", zipped: true, mutate: removeConfigEntry, wantSubstr: "missing archive member"},
		{name: "tar duplicate", mutate: duplicateBinaryEntry, wantSubstr: "duplicate archive member"},
		{name: "zip duplicate", zipped: true, mutate: duplicateBinaryEntry, wantSubstr: "duplicate archive member"},
		{name: "tar traversal", mutate: addTraversalEntry, wantSubstr: "unsafe archive path"},
		{name: "zip traversal", zipped: true, mutate: addTraversalEntry, wantSubstr: "unsafe archive path"},
		{name: "tar mode", mutate: weakenBinaryMode, wantSubstr: "has mode 0644"},
		{name: "zip mode", zipped: true, mutate: weakenBinaryMode, wantSubstr: "has mode 0644"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			item := target{os: "linux", arch: "amd64", zip: test.zipped}
			if test.zipped {
				item.os = "windows"
			}

			releaseDir := t.TempDir()
			entries := test.mutate(item, validTestEntries(item))
			writeTestBundle(t, releaseDir, item, entries)

			err := verifyBundle(
				releaseDir,
				testVersion,
				"test-commit",
				item,
				func(string, string, string, string) error { return nil },
			)
			if err == nil || !strings.Contains(err.Error(), test.wantSubstr) {
				t.Fatalf("verify error = %v, want %q", err, test.wantSubstr)
			}
		})
	}
}

func TestVerifyAllRejectsTamperedBundleChecksum(t *testing.T) {
	t.Parallel()

	repositoryRoot := newTestRepository(t, false)
	releaseDir := newTestRelease(t, testVersion)
	if err := CreateAll(repositoryRoot, releaseDir, testVersion); err != nil {
		t.Fatalf("create release: %v", err)
	}

	bundlePath := filepath.Join(releaseDir, releaseTargets[0].bundleName(testVersion))
	file, err := os.OpenFile(bundlePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	if _, err := file.WriteString("tampered"); err != nil {
		t.Fatalf("tamper bundle: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close bundle: %v", err)
	}

	err = verifyAll(
		releaseDir,
		testVersion,
		"test-commit",
		func(string, string, string, string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("verify error = %v, want checksum mismatch", err)
	}
}

func TestCreateAllRejectsMissingBinaryAndExistingBundle(t *testing.T) {
	t.Parallel()

	repositoryRoot := newTestRepository(t, false)
	releaseDir := newTestRelease(t, testVersion)
	missing := filepath.Join(releaseDir, releaseTargets[0].binaryAsset("torrctl", testVersion))
	if err := os.Remove(missing); err != nil {
		t.Fatalf("remove test binary: %v", err)
	}

	if err := CreateAll(repositoryRoot, releaseDir, testVersion); err == nil {
		t.Fatal("CreateAll accepted a missing binary")
	}

	releaseDir = newTestRelease(t, testVersion)
	existing := filepath.Join(releaseDir, releaseTargets[0].bundleName(testVersion))
	if err := os.WriteFile(existing, []byte("stale"), configMode); err != nil {
		t.Fatalf("write stale bundle: %v", err)
	}

	if err := CreateAll(repositoryRoot, releaseDir, testVersion); err == nil {
		t.Fatal("CreateAll accepted an existing bundle")
	}
}

func TestVerifyBundleRejectsDebugEnabledConfig(t *testing.T) {
	t.Parallel()

	item := target{os: "linux", arch: "amd64"}
	entries := validTestEntries(item)
	for index := range entries {
		if strings.HasSuffix(entries[index].name, "config.example.yml") {
			entries[index].data = []byte("debug:\n  enabled: true\n")
		}
	}

	releaseDir := t.TempDir()
	writeTestBundle(t, releaseDir, item, entries)
	err := verifyBundle(
		releaseDir,
		testVersion,
		"test-commit",
		item,
		func(string, string, string, string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "debug.enabled disabled") {
		t.Fatalf("verify error = %v, want release-safe config rejection", err)
	}
}

func newTestRepository(t *testing.T, debugEnabled bool) string {
	t.Helper()

	root := t.TempDir()
	configDir := filepath.Join(root, "server", "config")
	if err := os.MkdirAll(configDir, directoryMode); err != nil {
		t.Fatalf("create config directory: %v", err)
	}

	enabled := "false"
	if debugEnabled {
		enabled = "true"
	}

	if err := os.WriteFile(
		filepath.Join(configDir, "config.yml"),
		[]byte("server:\n  port: \"8090\"\ndebug:\n  enabled: "+enabled+"\n"),
		configMode,
	); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return root
}

func newTestRelease(t *testing.T, version string) string {
	t.Helper()

	releaseDir := t.TempDir()
	for _, item := range releaseTargets {
		for _, product := range []string{"torrserver", "torrctl"} {
			name := item.binaryAsset(product, version)
			content := []byte(product + "-" + item.platform() + "\n")
			if err := os.WriteFile(filepath.Join(releaseDir, name), content, binaryMode); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
	}

	return releaseDir
}

func validTestEntries(item target) []archiveEntry {
	topLevel := item.topLevel(testVersion)
	entries := []archiveEntry{
		{name: topLevel + "/", mode: directoryMode, directory: true},
		{name: topLevel + "/config.example.yml", mode: configMode, data: []byte("debug:\n  enabled: false\n")},
		{name: topLevel + "/" + item.bundledBinary("torrctl"), mode: binaryMode, data: []byte("torrctl")},
		{name: topLevel + "/" + item.bundledBinary("torrserver"), mode: binaryMode, data: []byte("torrserver")},
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].name < entries[right].name
	})

	return entries
}

func removeConfigEntry(_ target, entries []archiveEntry) []archiveEntry {
	for index, entry := range entries {
		if strings.HasSuffix(entry.name, "config.example.yml") {
			return append(entries[:index:index], entries[index+1:]...)
		}
	}

	return entries
}

func duplicateBinaryEntry(_ target, entries []archiveEntry) []archiveEntry {
	for _, entry := range entries {
		if strings.Contains(entry.name, "torrserver") {
			return append(entries, entry)
		}
	}

	return entries
}

func addTraversalEntry(_ target, entries []archiveEntry) []archiveEntry {
	return append(entries, archiveEntry{name: "../credential", mode: configMode, data: []byte("secret")})
}

func weakenBinaryMode(item target, entries []archiveEntry) []archiveEntry {
	for index := range entries {
		if strings.HasSuffix(entries[index].name, "/"+item.bundledBinary("torrctl")) {
			entries[index].mode = configMode

			break
		}
	}

	return entries
}

func writeTestBundle(t *testing.T, releaseDir string, item target, entries []archiveEntry) {
	t.Helper()

	file, err := os.Create(filepath.Join(releaseDir, item.bundleName(testVersion)))
	if err != nil {
		t.Fatalf("create test bundle: %v", err)
	}

	var writeErr error
	if item.zip {
		writeErr = writeZIP(file, entries)
	} else {
		writeErr = writeTarGZ(file, entries)
	}

	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write test bundle: %v", errors.Join(writeErr, closeErr))
	}
}

func readTestFile(t *testing.T, filePath string) []byte {
	t.Helper()

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open %s: %v", filePath, err)
	}

	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read %s: %v", filePath, errors.Join(readErr, closeErr))
	}

	return data
}
