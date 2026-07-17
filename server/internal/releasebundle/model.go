// Package releasebundle creates and verifies the two-binary platform archives
// published by the TorrServerV2 release workflow.
package releasebundle

import (
	"fmt"
	"regexp"
	"sort"
)

const productName = "TorrServerV2"

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)\.[0-9]+)?$`)

type target struct {
	os   string
	arch string
	zip  bool
}

var releaseTargets = []target{
	{os: "darwin", arch: "amd64"},
	{os: "darwin", arch: "arm64"},
	{os: "linux", arch: "amd64"},
	{os: "linux", arch: "arm64"},
	{os: "windows", arch: "amd64", zip: true},
}

func validateVersion(version string) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid release version: %s", version)
	}

	return nil
}

func (item target) platform() string {
	return item.os + "-" + item.arch
}

func (item target) topLevel(version string) string {
	return fmt.Sprintf("%s-v%s-%s", productName, version, item.platform())
}

func (item target) bundleName(version string) string {
	extension := ".tar.gz"
	if item.zip {
		extension = ".zip"
	}

	return item.topLevel(version) + extension
}

func (item target) binaryAsset(product, version string) string {
	extension := ""
	if item.zip {
		extension = ".exe"
	}

	return fmt.Sprintf("%s-%s-%s%s", product, version, item.platform(), extension)
}

func (item target) bundledBinary(product string) string {
	if item.zip {
		return product + ".exe"
	}

	return product
}

func expectedAssetNames(version string) []string {
	names := make([]string, 0, len(releaseTargets)*3)

	for _, item := range releaseTargets {
		names = append(names, item.binaryAsset("torrserver", version))
		names = append(names, item.binaryAsset("torrctl", version))
		names = append(names, item.bundleName(version))
	}

	sort.Strings(names)

	return names
}
