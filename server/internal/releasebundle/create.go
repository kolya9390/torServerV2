package releasebundle

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	directoryMode = os.FileMode(0o755)
	binaryMode    = os.FileMode(0o755)
	configMode    = os.FileMode(0o644)
)

var (
	tarTimestamp = time.Unix(0, 0).UTC()
	zipTimestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
)

type archiveEntry struct {
	name      string
	mode      os.FileMode
	data      []byte
	directory bool
}

// CreateAll creates one deterministic platform bundle and an aggregate
// checksum manifest for an already collected release binary matrix.
func CreateAll(repositoryRoot, releaseDir, version string) error {
	if err := validateVersion(version); err != nil {
		return err
	}

	configPath := filepath.Join(repositoryRoot, "server", "config", "config.yml")

	config, err := readRegularFile(configPath)
	if err != nil {
		return fmt.Errorf("read release config example: %w", err)
	}

	if err := validateCreateInputs(releaseDir, version); err != nil {
		return err
	}

	for _, item := range releaseTargets {
		if err := createBundle(releaseDir, version, item, config); err != nil {
			return err
		}
	}

	if err := writeChecksumManifest(releaseDir, version); err != nil {
		return fmt.Errorf("write release checksums: %w", err)
	}

	return nil
}

func validateCreateInputs(releaseDir, version string) error {
	info, err := os.Stat(releaseDir)
	if err != nil {
		return fmt.Errorf("inspect release directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("release path is not a directory: %s", releaseDir)
	}

	for _, item := range releaseTargets {
		bundlePath := filepath.Join(releaseDir, item.bundleName(version))
		if _, err := os.Lstat(bundlePath); err == nil {
			return fmt.Errorf("release bundle already exists: %s", bundlePath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect release bundle: %w", err)
		}

		for _, product := range []string{"torrserver", "torrctl"} {
			name := item.binaryAsset(product, version)
			if err := validateRegularFile(filepath.Join(releaseDir, name)); err != nil {
				return fmt.Errorf("validate %s: %w", name, err)
			}
		}
	}

	return nil
}

func createBundle(releaseDir, version string, item target, config []byte) error {
	topLevel := item.topLevel(version)
	entries := []archiveEntry{
		{name: topLevel + "/", mode: directoryMode, directory: true},
		{name: topLevel + "/config.example.yml", mode: configMode, data: config},
	}

	for _, product := range []string{"torrctl", "torrserver"} {
		sourceName := item.binaryAsset(product, version)

		data, err := readRegularFile(filepath.Join(releaseDir, sourceName))
		if err != nil {
			return fmt.Errorf("read %s: %w", sourceName, err)
		}

		entries = append(entries, archiveEntry{
			name: topLevel + "/" + item.bundledBinary(product),
			mode: binaryMode,
			data: data,
		})
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].name < entries[right].name
	})

	destinationName := item.bundleName(version)

	if item.zip {
		return writeAtomic(releaseDir, destinationName, func(writer io.Writer) error {
			return writeZIP(writer, entries)
		})
	}

	return writeAtomic(releaseDir, destinationName, func(writer io.Writer) error {
		return writeTarGZ(writer, entries)
	})
}

func readRegularFile(filePath string) ([]byte, error) {
	if err := validateRegularFile(filePath); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func validateRegularFile(filePath string) error {
	info, err := os.Lstat(filePath)
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", filePath)
	}

	if info.Size() == 0 {
		return fmt.Errorf("empty file: %s", filePath)
	}

	return nil
}

func writeAtomic(releaseDir, destinationName string, write func(io.Writer) error) (returnErr error) {
	if !filepath.IsLocal(destinationName) || filepath.Base(destinationName) != destinationName {
		return fmt.Errorf("invalid release asset name: %q", destinationName)
	}

	releaseRoot, err := os.OpenRoot(releaseDir)
	if err != nil {
		return fmt.Errorf("open release directory: %w", err)
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(releaseRoot, "close release directory"))
	}()

	temporary, err := os.CreateTemp(releaseDir, ".release-bundle-*")
	if err != nil {
		return fmt.Errorf("create temporary bundle: %w", err)
	}

	temporaryName := filepath.Base(temporary.Name())
	closed := false

	defer func() {
		if !closed {
			closeErr := temporary.Close()
			if closeErr != nil {
				returnErr = errors.Join(returnErr, fmt.Errorf("close temporary bundle: %w", closeErr))
			}
		}

		if returnErr != nil {
			if removeErr := releaseRoot.Remove(temporaryName); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary bundle: %w", removeErr))
			}
		}
	}()

	if err := temporary.Chmod(configMode); err != nil {
		return fmt.Errorf("set bundle mode: %w", err)
	}

	buffered := bufio.NewWriter(temporary)
	if err := write(buffered); err != nil {
		return err
	}

	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush bundle: %w", err)
	}

	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync bundle: %w", err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close bundle before publish: %w", err)
	}

	closed = true

	if err := releaseRoot.Rename(temporaryName, destinationName); err != nil {
		return fmt.Errorf("publish release bundle: %w", err)
	}

	return nil
}

func writeTarGZ(destination io.Writer, entries []archiveEntry) error {
	gzipWriter, err := gzip.NewWriterLevel(destination, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("create gzip writer: %w", err)
	}

	gzipWriter.ModTime = tarTimestamp
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	for _, entry := range entries {
		header := &tar.Header{
			Name:    entry.name,
			Mode:    int64(entry.mode.Perm()),
			ModTime: tarTimestamp,
			Format:  tar.FormatUSTAR,
		}
		if entry.directory {
			header.Typeflag = tar.TypeDir
		} else {
			header.Typeflag = tar.TypeReg
			header.Size = int64(len(entry.data))
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("write tar header %s: %w", entry.name, err)
		}

		if !entry.directory {
			if _, err := tarWriter.Write(entry.data); err != nil {
				return fmt.Errorf("write tar member %s: %w", entry.name, err)
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}

	return nil
}

func writeZIP(destination io.Writer, entries []archiveEntry) error {
	zipWriter := zip.NewWriter(destination)

	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}

		header.Modified = zipTimestamp
		if entry.directory {
			header.Method = zip.Store
			header.SetMode(directoryMode | os.ModeDir)
		} else {
			header.SetMode(entry.mode)
		}

		member, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create zip member %s: %w", entry.name, err)
		}

		if !entry.directory {
			if _, err := member.Write(entry.data); err != nil {
				return fmt.Errorf("write zip member %s: %w", entry.name, err)
			}
		}
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("close zip writer: %w", err)
	}

	return nil
}

func writeChecksumManifest(releaseDir, version string) error {
	manifestName := "torrserver-" + version + "-SHA256SUMS"

	return writeAtomic(releaseDir, manifestName, func(writer io.Writer) error {
		for _, name := range expectedAssetNames(version) {
			file, err := os.Open(filepath.Join(releaseDir, name))
			if err != nil {
				return fmt.Errorf("open checksum asset %s: %w", name, err)
			}

			digest := sha256.New()
			_, copyErr := io.Copy(digest, file)

			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}

			if _, err := fmt.Fprintf(writer, "%x  %s\n", digest.Sum(nil), name); err != nil {
				return fmt.Errorf("write checksum for %s: %w", name, err)
			}
		}

		return nil
	})
}
