// Package macos holds what the macOS packagers (app, dmg and pkg) share:
// architecture names, path comparison and the handling of shared settings
// macOS has no place for.
package macos

import (
	"fmt"
	"strings"
)

// The architectures a macOS package can be built for.
const (
	ArchARM64     = "arm64"
	ArchX8664     = "x86_64"
	ArchUniversal = "universal"
)

// nolint: gochecknoglobals
var archToMacOS = map[string]string{
	"amd64":     ArchX8664,
	"x86_64":    ArchX8664,
	"arm64":     ArchARM64,
	"aarch64":   ArchARM64,
	"all":       ArchUniversal,
	"universal": ArchUniversal,
}

// ResolveArch returns the architecture a package is built for, in macOS
// nomenclature. An explicit format-specific value (set through field, for
// example "pkg.arch") wins and must already be one of the macOS names;
// otherwise the shared arch is mapped. macOS runs on exactly two
// architectures, so anything else is an error instead of a package that
// declares the wrong one.
func ResolveArch(field, explicit, shared string) (string, error) {
	if explicit != "" {
		switch explicit {
		case ArchARM64, ArchX8664, ArchUniversal:
			return explicit, nil
		default:
			return "", fmt.Errorf(
				"package %s %q is not supported: use %s, %s or %s",
				field, explicit, ArchARM64, ArchX8664, ArchUniversal)
		}
	}
	if arch, ok := archToMacOS[strings.ToLower(shared)]; ok {
		return arch, nil
	}
	return "", fmt.Errorf(
		"package arch %q is not supported by macOS: use amd64, arm64 or all, or set %s",
		shared, field)
}

// HostArchitectures lists the machine architectures an (already resolved)
// package architecture runs on.
func HostArchitectures(arch string) []string {
	if arch == ArchUniversal {
		return []string{ArchARM64, ArchX8664}
	}
	return []string{arch}
}
