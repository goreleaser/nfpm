package msi

import (
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
)

// identitySeed preserves the original GUID contract for ordinary fields, while
// putting ambiguous tuples in a separate, length-framed namespace. Escaping
// only a separator would itself collide with literal escape characters.
func identitySeed(fields ...string) string {
	for _, field := range fields {
		if strings.Contains(field, "|") {
			var b strings.Builder
			b.WriteString("framed:")
			for _, value := range fields {
				fmt.Fprintf(&b, "%d:%s", len(value), value)
			}
			return b.String()
		}
	}
	return strings.Join(fields, "|")
}

func validateVersionFields(version string) error {
	core := strings.TrimPrefix(version, "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) > 4 {
		return fmt.Errorf("version must have at most four numeric fields")
	}
	for _, field := range parts {
		if field == "" || strings.IndexFunc(field, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return fmt.Errorf("version field %q must be numeric", field)
		}
		if _, err := strconv.ParseUint(field, 10, 63); err != nil {
			return fmt.Errorf("version field %q overflows the Windows Installer version limits", field)
		}
	}
	return nil
}

func firstLicense(contents files.Contents) string {
	var first string
	for _, c := range contents {
		if c.Packager != "" && c.Packager != packagerName {
			continue
		}
		if c.Type != files.TypeRPMLicense && c.Type != files.TypeRPMLicence {
			continue
		}
		if first == "" {
			first = c.Source
		} else {
			log.Printf("warning: multiple license contents, %s is not used for the install UI", c.Source)
		}
	}
	return first
}

func validatePathTree(info *nfpm.Info, filePaths map[string]string) error {
	directories := map[string]bool{}
	for target, source := range filePaths {
		for parent := path.Dir(target); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if previous, ok := filePaths[parent]; ok {
				return fmt.Errorf("package contents %q is a file but also a directory for %q", previous, source)
			}
			directories[parent] = true
		}
	}
	for _, c := range info.Contents {
		if c.Type != files.TypeDir {
			continue
		}
		// Model a directory by resolving an imaginary child. This also handles
		// an exact standard-folder prefix without treating its name as a file.
		target := path.Dir(resolvedDestinationKey(info, path.Join(normalizeDest(c.Destination), "_directory_child")))
		for parent := target; parent != "." && parent != "/"; parent = path.Dir(parent) {
			if previous, ok := filePaths[parent]; ok {
				return fmt.Errorf("package contents %q is a file but also a directory for %q", previous, c.Destination)
			}
		}
		if !directories[target] {
			return fmt.Errorf("package contents %q: msi does not support empty directories; remove the directory entry or include a file", c.Destination)
		}
	}
	return nil
}

// mapPackageDestination routes the product's explicit Program Files subtree
// through INSTALLFOLDER too. This gives its files, shortcuts, service and
// registry references the same target when an installer redirects the folder.
func mapPackageDestination(info *nfpm.Info, dest string) (root, defaultDir, rel string) {
	root, defaultDir, rel = mapDestination(dest, is64bit(info.Arch))
	productRoot := "ProgramFilesFolder"
	if is64bit(info.Arch) {
		productRoot = "ProgramFiles64Folder"
	}
	if !info.MSI.PerUser && root == productRoot {
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], info.MSI.InstallDir) {
			return installFolder, "", parts[1]
		}
	}
	return root, defaultDir, rel
}

func registryResourceKey(info *nfpm.Info, r nfpm.MSIRegistry) string {
	root, key := strings.ToUpper(r.Root), strings.ToLower(r.Key)
	if root == "HKMU" || root == "HKCR" {
		if root == "HKCR" {
			key = `software\classes\` + key
		}
		root = "HKLM"
		if info.MSI.PerUser {
			root = "HKCU"
		}
	}
	return identitySeed(root, key, strings.ToLower(r.Name))
}

func validateResourceOwners(info *nfpm.Info) error {
	registry := map[string]int{}
	for n, r := range info.MSI.Registry {
		key := registryResourceKey(info, r)
		if previous, exists := registry[key]; exists {
			return fmt.Errorf("package msi.registry[%d] and msi.registry[%d] resolve to the same registry value", previous, n)
		}
		registry[key] = n
	}
	services := map[string]int{}
	for n, s := range info.MSI.Services {
		key := strings.ToLower(s.Name)
		if previous, exists := services[key]; exists {
			return fmt.Errorf("package msi.services[%d] and msi.services[%d] name the same Windows service %q", previous, n, s.Name)
		}
		services[key] = n
	}
	shortcuts := map[string]int{}
	for n, s := range info.MSI.Shortcuts {
		if info.MSI.PerUser && protectedShortcutDirectory(s.Directory) {
			return fmt.Errorf("package msi.shortcuts[%d].directory %q is protected and cannot be used by a non-elevated per-user install", n, s.Directory)
		}
		key := identitySeed(s.Directory, strings.ToLower(s.Name))
		if previous, exists := shortcuts[key]; exists {
			return fmt.Errorf("package msi.shortcuts[%d] and msi.shortcuts[%d] create the same Windows shortcut %q", previous, n, s.Name)
		}
		shortcuts[key] = n
	}
	return nil
}

func protectedShortcutDirectory(dir string) bool {
	switch dir {
	case "WindowsFolder", "WindowsVolume", "SystemFolder", "System64Folder", "FontsFolder", "CommonAppDataFolder":
		return true
	default:
		return false
	}
}
