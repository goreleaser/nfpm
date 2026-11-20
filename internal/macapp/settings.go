// Package macapp builds the macOS application bundle that the app and dmg
// packagers share. Both describe the same bundle and only differ in the
// container they deliver it in.
package macapp

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/goreleaser/nfpm/v2"
)

// nolint: gochecknoglobals
var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
	// dottedNumbers matches one to three numbers separated by periods, the
	// form of both a macOS version and a bundle's build version.
	dottedNumbers = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(\.[0-9]+)?$`)

	// reservedPlistKeys are the Info.plist keys nFPM writes from other
	// settings. Accepting them in app.info_plist would leave two sources for
	// one value.
	reservedPlistKeys = map[string]string{
		"CFBundleExecutable":            "app.executable",
		"CFBundleIdentifier":            "app.identifier",
		"CFBundleInfoDictionaryVersion": "",
		"CFBundleName":                  "app.name",
		"CFBundlePackageType":           "",
		"CFBundleShortVersionString":    "version",
		"CFBundleVersion":               "version",
		"LSMinimumSystemVersion":        "app.min_os_version",
	}
)

// BundleName returns the name of the bundle: app.name, or the package name.
func BundleName(info *nfpm.Info) string {
	if info.App.Name != "" {
		return info.App.Name
	}
	return info.Name
}

// executableName returns the name of the main executable in Contents/MacOS:
// app.executable, or the package name.
func executableName(info *nfpm.Info) string {
	if info.App.Executable != "" {
		return info.App.Executable
	}
	return info.Name
}

// Version returns the version shown for the application and used in file
// names. It keeps every part of the release identity.
func Version(info *nfpm.Info) string {
	version := info.Version
	if info.Prerelease != "" {
		version += "-" + info.Prerelease
	}
	if info.VersionMetadata != "" {
		version += "+" + info.VersionMetadata
	}
	if info.Release != "" {
		version += "-" + info.Release
	}
	return version
}

// validateSettings checks the settings that do not depend on the contents.
func validateSettings(info *nfpm.Info) error {
	if info.App.Identifier == "" {
		return errors.New("package app.identifier must be provided, for example com.example." + info.Name)
	}
	if !identifierPattern.MatchString(info.App.Identifier) {
		return fmt.Errorf(
			"package app.identifier %q is not valid: use letters, digits, hyphens and periods, for example com.example.myapp",
			info.App.Identifier)
	}
	if v := info.App.MinOSVersion; v != "" && !dottedNumbers.MatchString(v) {
		return fmt.Errorf("package app.min_os_version %q is not valid: use a macOS version such as 13.0", v)
	}
	// macOS compares CFBundleVersion numerically to tell releases apart, so a
	// version it cannot read must not be replaced by a made-up one.
	if !dottedNumbers.MatchString(info.Version) {
		return fmt.Errorf(
			"package version %q is not valid for a macOS application: it must be one to three numbers separated by periods",
			info.Version)
	}
	if err := validateFileName("app.name", BundleName(info)); err != nil {
		return err
	}
	if err := validateFileName("app.executable", executableName(info)); err != nil {
		return err
	}
	return validatePlist(info.App.InfoPlist)
}

// validateFileName rejects a name that cannot be a single file name on a
// macOS volume.
func validateFileName(label, name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/:\x00") {
		return fmt.Errorf("package %s %q is not a valid file name on macOS", label, name)
	}
	return nil
}

// validatePlist rejects the keys nFPM owns and the values a property list
// cannot hold.
func validatePlist(plist map[string]any) error {
	keys := make([]string, 0, len(plist))
	for key := range plist {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if setting, reserved := reservedPlistKeys[key]; reserved {
			if setting == "" {
				return fmt.Errorf("package app.info_plist[%q] is set by nfpm and cannot be changed", key)
			}
			return fmt.Errorf("package app.info_plist[%q] is set by nfpm; use %s instead", key, setting)
		}
		if err := validatePlistValue(fmt.Sprintf("app.info_plist[%q]", key), plist[key]); err != nil {
			return err
		}
	}
	return nil
}

func validatePlistValue(where string, value any) error {
	switch typed := value.(type) {
	case string, bool, int, int64, uint64, float64, time.Time:
		return nil
	case []any:
		for n, item := range typed {
			if err := validatePlistValue(fmt.Sprintf("%s[%d]", where, n), item); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := validatePlistValue(fmt.Sprintf("%s[%q]", where, key), typed[key]); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf(
			"package %s has a value a property list cannot hold (%T): use text, numbers, booleans, dates, lists and maps with text keys",
			where, value)
	}
}
