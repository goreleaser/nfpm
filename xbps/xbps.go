// Package xbps implements nfpm.Packager providing .xbps bindings.
package xbps

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
)

const packagerName = "xbps"

var errPackageWriterNotImplemented = errors.New("xbps: package writer is not implemented")

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// nolint: gochecknoglobals
var archToXBPS = map[string]string{
	"all":     "noarch",
	"noarch":  "noarch",
	"amd64":   "x86_64",
	"x86_64":  "x86_64",
	"386":     "i686",
	"i386":    "i686",
	"i686":    "i686",
	"arm64":   "aarch64",
	"aarch64": "aarch64",
	"arm6":    "armv6l",
	"arm7":    "armv7l",
}

// Default XBPS packager.
// nolint: gochecknoglobals
var Default = &XBPS{}

// XBPS packager implementation.
type XBPS struct{}

func ensureValidArch(info *nfpm.Info) (*nfpm.Info, error) {
	if info.XBPS.Arch != "" {
		info.Arch = info.XBPS.Arch
		return info, nil
	}

	arch, ok := archToXBPS[info.Arch]
	if !ok {
		return nil, fmt.Errorf("xbps: unsupported architecture %q", info.Arch)
	}
	info.Arch = arch
	return info, nil
}

func normalizeVersionPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "-")
	value = strings.Trim(value, ".")
	return value
}

func version(info *nfpm.Info) string {
	base := strings.TrimSpace(info.Version)
	base = strings.TrimPrefix(base, "v")

	parts := []string{base}
	if pre := normalizeVersionPart(info.Prerelease); pre != "" {
		parts = append(parts, pre)
	}
	if meta := normalizeVersionPart(info.VersionMetadata); meta != "" {
		parts = append(parts, meta)
	}
	return strings.Join(parts, ".")
}

func revision(info *nfpm.Info) (string, error) {
	trimmed := strings.TrimSpace(info.Release)
	if trimmed == "" {
		return "1", nil
	}

	rev, err := strconv.Atoi(trimmed)
	if err != nil || rev < 1 {
		return "", fmt.Errorf("xbps: release %q must be a positive integer revision", info.Release)
	}
	return trimmed, nil
}

func pkgver(info *nfpm.Info) (string, error) {
	rev, err := revision(info)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s_%s", info.Name, version(info), rev), nil
}

// ConventionalFileName returns a file name according to XBPS package conventions.
func (*XBPS) ConventionalFileName(info *nfpm.Info) string {
	copyInfo := *info
	normalized, err := ensureValidArch(&copyInfo)
	if err != nil {
		normalized = &copyInfo
	}

	pv, err := pkgver(normalized)
	if err != nil {
		pv = fmt.Sprintf("%s-%s_1", info.Name, version(info))
	}
	return fmt.Sprintf("%s.%s.xbps", pv, normalized.Arch)
}

// ConventionalExtension returns the file name conventionally used for XBPS packages.
func (*XBPS) ConventionalExtension() string {
	return ".xbps"
}

// Package currently validates the XBPS skeleton inputs and returns a placeholder until native writing lands.
func (*XBPS) Package(info *nfpm.Info, _ io.Writer) error {
	if info.Platform != "linux" {
		return fmt.Errorf("invalid platform: %s", info.Platform)
	}
	if _, err := ensureValidArch(info); err != nil {
		return err
	}
	if _, err := pkgver(info); err != nil {
		return err
	}
	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}
	return errPackageWriterNotImplemented
}
