package releasebundle

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	maxArchiveMembers = 8
	maxMemberBytes    = int64(256 << 20)
)

type archiveMember struct {
	name      string
	mode      fs.FileMode
	data      []byte
	directory bool
}

type binaryInspector func(filePath, product, version, commit string) error

// VerifyAll verifies the aggregate manifest, archive safety and metadata of
// both binaries in every platform bundle.
func VerifyAll(releaseDir, version, commit string) error {
	return verifyAll(releaseDir, version, commit, inspectBuildInfo)
}

func verifyAll(releaseDir, version, commit string, inspect binaryInspector) error {
	if err := validateVersion(version); err != nil {
		return err
	}

	if strings.TrimSpace(commit) == "" {
		return errors.New("release commit is required")
	}

	if err := verifyChecksumManifest(releaseDir, version); err != nil {
		return err
	}

	for _, item := range releaseTargets {
		if err := verifyBundle(releaseDir, version, commit, item, inspect); err != nil {
			return fmt.Errorf("verify %s: %w", item.bundleName(version), err)
		}
	}

	return nil
}

func verifyBundle(
	releaseDir,
	version,
	commit string,
	item target,
	inspect binaryInspector,
) (returnErr error) {
	archivePath := filepath.Join(releaseDir, item.bundleName(version))

	members, err := readArchive(archivePath, item.zip)
	if err != nil {
		return err
	}

	expected := expectedMembers(item, version)
	seen := make(map[string]struct{}, len(members))

	temporaryDir, err := os.MkdirTemp("", "torrserver-release-verify-*")
	if err != nil {
		return fmt.Errorf("create verification directory: %w", err)
	}

	defer func() {
		if removeErr := os.RemoveAll(temporaryDir); removeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("remove verification directory: %w", removeErr))
		}
	}()

	for _, member := range members {
		if err := verifyArchiveMember(member, expected, seen, temporaryDir, version, commit, inspect); err != nil {
			return err
		}
	}

	return verifyExpectedMembers(seen, expected)
}

type memberRequirement struct {
	mode      fs.FileMode
	directory bool
	config    bool
	product   string
}

func verifyArchiveMember(
	member archiveMember,
	expected map[string]memberRequirement,
	seen map[string]struct{},
	temporaryDir,
	version,
	commit string,
	inspect binaryInspector,
) error {
	if err := validateArchivePath(member.name); err != nil {
		return err
	}

	if _, ok := seen[member.name]; ok {
		return fmt.Errorf("duplicate archive member: %s", member.name)
	}

	requirement, ok := expected[member.name]
	if !ok {
		return fmt.Errorf("unexpected archive member: %s", member.name)
	}

	seen[member.name] = struct{}{}

	if member.directory != requirement.directory {
		return fmt.Errorf("archive member type mismatch: %s", member.name)
	}

	if member.mode.Perm() != requirement.mode.Perm() {
		return fmt.Errorf(
			"archive member %s has mode %04o; expected %04o",
			member.name,
			member.mode.Perm(),
			requirement.mode.Perm(),
		)
	}

	if requirement.product != "" {
		binaryPath := filepath.Join(temporaryDir, requirement.product)
		if err := os.WriteFile(binaryPath, member.data, binaryMode); err != nil {
			return fmt.Errorf("extract %s: %w", member.name, err)
		}

		if err := inspect(binaryPath, requirement.product, version, commit); err != nil {
			return fmt.Errorf("inspect %s: %w", member.name, err)
		}
	}

	if requirement.config && !bytes.Contains(member.data, []byte("debug:\n  enabled: false")) {
		return errors.New("config.example.yml does not keep debug.enabled disabled")
	}

	return nil
}

func verifyExpectedMembers(seen map[string]struct{}, expected map[string]memberRequirement) error {
	if len(seen) == len(expected) {
		return nil
	}

	for name := range expected {
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("missing archive member: %s", name)
		}
	}

	return nil
}

func expectedMembers(item target, version string) map[string]memberRequirement {
	topLevel := item.topLevel(version)

	return map[string]memberRequirement{
		topLevel + "/":                   {mode: directoryMode, directory: true},
		topLevel + "/config.example.yml": {mode: configMode, config: true},
		topLevel + "/" + item.bundledBinary("torrctl"): {
			mode: binaryMode, product: "torrctl",
		},
		topLevel + "/" + item.bundledBinary("torrserver"): {
			mode: binaryMode, product: "torrserver",
		},
	}
}

func validateArchivePath(name string) error {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) {
		return fmt.Errorf("unsafe archive path: %q", name)
	}

	cleaned := path.Clean(strings.TrimSuffix(name, "/"))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("unsafe archive path: %q", name)
	}

	if cleaned != strings.TrimSuffix(name, "/") {
		return fmt.Errorf("non-canonical archive path: %q", name)
	}

	return nil
}

func readArchive(archivePath string, zipped bool) ([]archiveMember, error) {
	if zipped {
		return readZIP(archivePath)
	}

	return readTarGZ(archivePath)
}

func readTarGZ(archivePath string) (members []archiveMember, returnErr error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(file, "close archive"))
	}()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("open gzip stream: %w", err)
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(gzipReader, "close gzip stream"))
	}()

	tarReader := tar.NewReader(gzipReader)
	members = make([]archiveMember, 0, 4)

	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}

		if nextErr != nil {
			return nil, fmt.Errorf("read tar member: %w", nextErr)
		}

		if len(members) >= maxArchiveMembers {
			return nil, errors.New("archive contains too many members")
		}

		member := archiveMember{name: header.Name, mode: fs.FileMode(header.Mode)}
		switch header.Typeflag {
		case tar.TypeDir:
			member.directory = true
		case tar.TypeReg, tar.TypeRegA:
			data, readErr := readBounded(tarReader, header.Size)
			if readErr != nil {
				return nil, fmt.Errorf("read tar member %s: %w", header.Name, readErr)
			}

			member.data = data
		default:
			return nil, fmt.Errorf("unsupported archive member type for %s", header.Name)
		}

		members = append(members, member)
	}

	return members, nil
}

func readZIP(archivePath string) (members []archiveMember, returnErr error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open zip archive: %w", err)
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(reader, "close zip archive"))
	}()

	if len(reader.File) > maxArchiveMembers {
		return nil, errors.New("archive contains too many members")
	}

	members = make([]archiveMember, 0, len(reader.File))
	for _, file := range reader.File {
		mode := file.Mode()

		member := archiveMember{name: file.Name, mode: mode, directory: file.FileInfo().IsDir()}
		if !member.directory {
			if !mode.IsRegular() {
				return nil, fmt.Errorf("unsupported archive member type for %s", file.Name)
			}

			stream, openErr := file.Open()
			if openErr != nil {
				return nil, fmt.Errorf("open zip member %s: %w", file.Name, openErr)
			}

			data, readErr := readBounded(stream, int64(file.UncompressedSize64))

			closeErr := stream.Close()
			if readErr != nil || closeErr != nil {
				return nil, errors.Join(readErr, closeErr)
			}

			member.data = data
		}

		members = append(members, member)
	}

	return members, nil
}

func closeWithContext(closer io.Closer, operation string) error {
	if err := closer.Close(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func readBounded(reader io.Reader, declaredSize int64) ([]byte, error) {
	if declaredSize < 0 || declaredSize > maxMemberBytes {
		return nil, fmt.Errorf("archive member size %d exceeds limit", declaredSize)
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxMemberBytes+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) != declaredSize {
		return nil, fmt.Errorf("archive member size mismatch: read %d, expected %d", len(data), declaredSize)
	}

	return data, nil
}

func inspectBuildInfo(filePath, product, version, commit string) error {
	info, err := buildinfo.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read Go build info: %w", err)
	}

	if info.Path != "server/cmd/"+product {
		return fmt.Errorf("binary package is %q; expected server/cmd/%s", info.Path, product)
	}

	var linkerFlags string

	for _, setting := range info.Settings {
		if setting.Key == "-ldflags" {
			linkerFlags = setting.Value

			break
		}
	}

	required := []string{
		"-X=server/version.version=v" + version,
		"-X=server/version.commit=" + commit,
		"-X=server/version.buildTime=unknown",
		"-X=server/version.dirtyState=clean",
	}
	for _, value := range required {
		if !strings.Contains(linkerFlags, value) {
			return fmt.Errorf("binary is missing build metadata %q", value)
		}
	}

	return nil
}

func verifyChecksumManifest(releaseDir, version string) (returnErr error) {
	manifestName := "torrserver-" + version + "-SHA256SUMS"

	releaseRoot, err := os.OpenRoot(releaseDir)
	if err != nil {
		return fmt.Errorf("open release directory: %w", err)
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(releaseRoot, "close release directory"))
	}()

	file, err := releaseRoot.Open(manifestName)
	if err != nil {
		return fmt.Errorf("open checksum manifest: %w", err)
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(file, "close checksum manifest"))
	}()

	expected := expectedAssetNames(version)
	index := 0
	seen := make(map[string]struct{}, len(expected))

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			return fmt.Errorf("invalid checksum manifest line: %q", scanner.Text())
		}

		name := fields[1]
		if index >= len(expected) || name != expected[index] {
			return fmt.Errorf("checksum manifest is incomplete or not deterministically ordered at %s", name)
		}

		index++

		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate checksum entry: %s", name)
		}

		seen[name] = struct{}{}

		digest, hashErr := checksumFile(releaseRoot, name)
		if hashErr != nil {
			return fmt.Errorf("hash checksummed asset %s: %w", name, hashErr)
		}

		if digest != strings.ToLower(fields[0]) {
			return fmt.Errorf("checksum mismatch: %s", name)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksum manifest: %w", err)
	}

	if index != len(expected) {
		return fmt.Errorf("checksum manifest contains %d entries; expected %d", index, len(expected))
	}

	return nil
}

func checksumFile(releaseRoot *os.Root, name string) (returnDigest string, returnErr error) {
	file, err := releaseRoot.Open(name)
	if err != nil {
		return "", err
	}

	defer func() {
		returnErr = errors.Join(returnErr, closeWithContext(file, "close checksummed asset"))
	}()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}
