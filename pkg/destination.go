package pkg

import (
	"fmt"
	"path"
	"strings"
)

// rootAliases are the root-level symbolic links macOS ships with. A payload
// path is written below the real directory so installing never depends on how
// the installer treats a symbolic link in the path.
// nolint: gochecknoglobals
var rootAliases = map[string]string{
	"/etc": "/private/etc",
	"/tmp": "/private/tmp",
	"/var": "/private/var",
}

// installRoots are the locations below which a package can install files. The
// rest of the root volume is either sealed (the read-only system volume) or
// protected by System Integrity Protection, so installing there fails.
// nolint: gochecknoglobals
var installRoots = []string{
	"/Applications",
	"/Library",
	"/Users",
	"/opt",
	"/private",
	"/usr/libexec/cups",
	"/usr/local",
	"/usr/share/snmp",
}

// canonicalDestination returns the path a contents destination is installed
// at: cleaned, absolute, and with a leading root-level symbolic link replaced
// by the directory it points to.
func canonicalDestination(destination string) string {
	clean := path.Clean("/" + strings.ReplaceAll(destination, "\\", "/"))
	for alias, target := range rootAliases {
		if clean == alias {
			return target
		}
		if rest, ok := strings.CutPrefix(clean, alias+"/"); ok {
			return target + "/" + rest
		}
	}
	return clean
}

// validateDestination rejects a (canonical) destination macOS would refuse at
// install time, when the cause is much harder to find.
func validateDestination(destination, canonical string) error {
	for _, target := range rootAliases {
		if canonical == target {
			return fmt.Errorf(
				"package contents %q: %s is a directory of macOS and cannot be replaced by a package; install below it instead",
				destination, target)
		}
	}
	for _, root := range installRoots {
		if strings.HasPrefix(canonical, root+"/") {
			return nil
		}
		if canonical == root {
			return fmt.Errorf(
				"package contents %q: %s is a directory of macOS and cannot be replaced by a package; install below it instead",
				destination, root)
		}
	}
	return fmt.Errorf(
		"package contents %q: macOS does not allow installing there; use a destination below one of %s "+
			"(/etc, /var and /tmp are also accepted), and keep other platforms' paths out with the contents \"packager\" field",
		destination, strings.Join(installRoots, ", "))
}

// isSystemDirectory reports whether a (canonical) path is one of the
// directories macOS itself provides: an install root, the target of a
// root-level symbolic link, or a parent of either. Every Mac has them, and the
// installer leaves an existing directory exactly as it is, so a package has
// nothing to say about them.
func isSystemDirectory(canonical string) bool {
	system := append([]string{}, installRoots...)
	for _, target := range rootAliases {
		system = append(system, target)
	}
	for _, directory := range system {
		if canonical == directory || strings.HasPrefix(directory, canonical+"/") {
			return true
		}
	}
	return false
}
