// Package msi implements nfpm.Packager providing real Windows Installer (.msi)
// builds via the pure-Go go.digitalxero.dev/go-msi library.
package msi

import (
	"cmp"
	"crypto/sha1"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/maintainer"
	"go.digitalxero.dev/go-msi"
)

const packagerName = "msi"

// mainFeature is the single primary feature every installed component is
// associated with.
const mainFeature = "MainFeature"

// installFolder is the Windows Installer directory the package's own files
// are installed under. It lives under Program Files for per-machine installs
// and under the user's local application data for per-user installs, and can
// be redirected with "msiexec INSTALLFOLDER=...".
const installFolder = "INSTALLFOLDER"

// perUserProgramsFolder is the directory ID for "%LOCALAPPDATA%\Programs", the
// conventional parent of per-user installs.
const perUserProgramsFolder = "LocalProgramsFolder"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// Default msi packager.
// nolint: gochecknoglobals
var Default = &MSI{}

// MSI is an msi packager implementation.
type MSI struct{}

// nolint: gochecknoglobals
var archToMSI = map[string]string{
	"amd64":   "x64",
	"x86_64":  "x64",
	"386":     "x86",
	"i386":    "x86",
	"i686":    "x86",
	"arm64":   "arm64",
	"aarch64": "arm64",
	"arm":     "arm",
	"arm7":    "arm",
	"ia64":    "intel64",
}

// ensureValidArch resolves the architecture the package is built for: an
// explicit msi.arch wins, otherwise the shared arch is mapped onto Windows
// Installer's names. The result is lowercased so "X64" and "x64" produce the
// same file name and, more importantly, the same derived GUIDs.
func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	if info.MSI.Arch != "" {
		info.Arch = strings.ToLower(info.MSI.Arch)
	} else if arch, ok := archToMSI[info.Arch]; ok {
		info.Arch = arch
	}
	return info
}

// msiPlatforms maps an MSI architecture name to the Windows Installer platform
// recorded in the package. These are the only architectures Windows Installer
// supports, so an architecture missing from this map cannot be packaged as an
// MSI at all. Note that Intel64 means Itanium, not x86-64.
// nolint: gochecknoglobals
var msiPlatforms = map[string]msi.Platform{
	"x86":     msi.Platform_Intel,
	"intel":   msi.Platform_Intel,
	"x64":     msi.Platform_x64,
	"arm":     msi.Platform_Arm,
	"arm64":   msi.Platform_Arm64,
	"intel64": msi.Platform_Intel64,
}

// platformFor resolves an (already MSI-normalized) architecture to its Windows
// Installer platform.
func platformFor(arch string) (msi.Platform, bool) {
	p, ok := msiPlatforms[strings.ToLower(arch)]
	return p, ok
}

// is64bit reports whether the (already MSI-normalized) architecture is 64-bit.
func is64bit(arch string) bool {
	switch p, _ := platformFor(arch); p {
	case msi.Platform_x64, msi.Platform_Arm64, msi.Platform_Intel64:
		return true
	default:
		return false
	}
}

// ConventionalFileName returns the conventional file name for an MSI package.
func (m *MSI) ConventionalFileName(info *nfpm.Info) string {
	info = ensureValidArch(info)
	return fmt.Sprintf("%s_%s_%s.msi", info.Name, msiVersion(info), info.Arch)
}

// ConventionalExtension returns the file extension for MSI packages.
func (*MSI) ConventionalExtension() string {
	return ".msi"
}

// SetPackagerDefaults sets default values for MSI-specific fields.
func (*MSI) SetPackagerDefaults(info *nfpm.Info) {
	if info.MSI.ProductName == "" {
		info.MSI.ProductName = info.Name
	}
	if info.MSI.Manufacturer == "" {
		info.MSI.Manufacturer = maintainer.VendorOrMaintainer(info.Vendor, info.Maintainer)
	}
	if info.MSI.InstallDir == "" {
		info.MSI.InstallDir = info.MSI.ProductName
	}
	for i := range info.MSI.Services {
		if info.MSI.Services[i].StartType == "" {
			info.MSI.Services[i].StartType = "demand"
		}
	}
	for i := range info.MSI.Shortcuts {
		if info.MSI.Shortcuts[i].Directory == "" {
			info.MSI.Shortcuts[i].Directory = "ProgramMenuFolder"
		}
	}
}

// Package writes a new MSI package to the given writer using the given info.
func (m *MSI) Package(info *nfpm.Info, w io.Writer) error {
	m.SetPackagerDefaults(info)
	info = ensureValidArch(info)

	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}

	if err := validate(info); err != nil {
		return err
	}

	// validate rejects an architecture with no Windows Installer platform and
	// a version outside the ProductVersion limits, so neither can fail here.
	platform, _ := platformFor(info.Arch)
	version := msiVersion(info)

	b := msi.NewPackage().
		WithProductName(info.MSI.ProductName).
		WithManufacturer(info.MSI.Manufacturer).
		WithVersion(version).
		WithPlatform(platform).
		WithAllUsers(!info.MSI.PerUser)

	// ProductCode must always be present. Windows Installer requires a new
	// ProductCode for every release for major upgrades to work, so the derived
	// default is namespaced by manufacturer, product name, architecture, and
	// the full release identity (including pre-release and metadata).
	// go-msi requires braced uppercase GUIDs; validation accepts either case, so
	// normalize the user-provided value here.
	productCode := strings.ToUpper(info.MSI.ProductCode)
	if productCode == "" {
		productCode = deriveProductCode(info)
	}
	b = b.WithProductCode(productCode)

	// UpgradeCode must stay stable across releases of the same product so the
	// Upgrade table can find older installs; namespace it by manufacturer,
	// product name, and architecture (per-arch products, no cross-arch
	// upgrades).
	upgradeCode := strings.ToUpper(info.MSI.UpgradeCode)
	if upgradeCode == "" {
		upgradeCode = deriveUpgradeCode(info)
	}
	b = b.WithUpgradeCode(upgradeCode)
	for k, v := range info.MSI.Properties {
		b = b.WithProperty(k, v)
	}

	// Map shared root metadata onto the standard ARP properties, unless the
	// user already set the property explicitly.
	for k, v := range map[string]string{
		"ARPCOMMENTS":     info.Description,
		"ARPURLINFOABOUT": info.Homepage,
	} {
		if _, ok := info.MSI.Properties[k]; v != "" && !ok {
			b = b.WithProperty(k, v)
		}
	}

	createdDirs := declareInstallFolder(b, info)

	// The single primary feature every component is associated with.
	b.Feature(mainFeature).WithTitle(info.MSI.ProductName).WithLevel(1)

	ids := newIDTable()
	placed, err := addContents(b, info, upgradeCode, ids, createdDirs)
	if err != nil {
		return err
	}

	if err := addShortcuts(b, info, placed, ids); err != nil {
		return err
	}
	if err := addServices(b, info, placed); err != nil {
		return err
	}
	if err := addRegistry(b, info, upgradeCode, ids); err != nil {
		return err
	}

	// Major upgrades are always on: every release replaces older installs of
	// the same upgrade code, which is what every other nfpm packager does by
	// default and what keeps a per-release ProductCode from leaving two
	// products side by side.
	mu := b.MajorUpgrade()
	if info.MSI.Upgrade.DowngradeErrorMessage != "" {
		mu.DowngradeErrorMessage(info.MSI.Upgrade.DowngradeErrorMessage)
	}
	if info.MSI.Upgrade.AllowDowngrades {
		mu.AllowDowngrades()
	}
	if info.MSI.Upgrade.AllowSameVersion {
		mu.AllowSameVersionUpgrades()
	}

	if err := addScripts(b, info); err != nil {
		return err
	}

	if info.MSI.MinimalUI {
		b.WithMinimalUI()
	}

	if info.MSI.Signature.PFXFile != "" {
		if err := configureSigning(b, info); err != nil {
			return err
		}
	}

	pkg, err := b.Build()
	if err != nil {
		return err
	}

	return pkg.WriteMSI(w)
}

// declareInstallFolder declares INSTALLFOLDER with the configured install
// directory name and parents it per the install scope: Program Files (the
// platform's 64- or 32-bit one) for per-machine installs, the user's
// "%LOCALAPPDATA%\Programs" for per-user installs. It returns the set of
// directory IDs already declared so addContents does not redeclare them.
func declareInstallFolder(b msi.PackageBuilder, info *nfpm.Info) map[string]bool {
	created := map[string]bool{installFolder: true}
	if info.MSI.PerUser {
		b.Directory("TARGETDIR").
			Subdirectory("LocalAppDataFolder", ".").
			Subdirectory(perUserProgramsFolder, "Programs").
			Subdirectory(installFolder, info.MSI.InstallDir)
		created["LocalAppDataFolder"] = true
		created[perUserProgramsFolder] = true
		return created
	}
	b.RootDirectory(installFolder, info.MSI.InstallDir)
	b.InstallToProgramFiles()
	return created
}

// placement records where a content file was installed so shortcuts and services
// can reference it by its original destination path.
type placement struct {
	componentID string
	rootID      string
}

// reservedProperties are the Property rows nfpm derives from its own fields;
// setting them through msi.properties would duplicate the Property primary
// key, so they are rejected up front with a pointer to the right field.
// nolint: gochecknoglobals
var reservedProperties = map[string]string{
	"ProductName":     "msi.product_name",
	"ProductVersion":  "msi.version",
	"Manufacturer":    "msi.manufacturer",
	"ProductCode":     "msi.product_code",
	"UpgradeCode":     "msi.upgrade_code",
	"ALLUSERS":        "msi.per_user",
	"ProductLanguage": "",
}

func validate(info *nfpm.Info) error {
	if info.MSI.Manufacturer == "" {
		return fmt.Errorf("package msi.manufacturer, vendor, or maintainer must be provided")
	}
	// The architecture is not just a label here: it is written to the package as
	// the target platform and decides where files land, so an architecture
	// Windows Installer has no platform for cannot produce a working package.
	if _, ok := platformFor(info.Arch); !ok {
		return fmt.Errorf(
			"package msi arch %q is not supported by Windows Installer, which targets only "+
				"x86, x64, arm, arm64, and intel64; set msi.arch to one of those",
			info.Arch)
	}
	if err := validateVersion(info); err != nil {
		return err
	}
	if err := validateScripts(info); err != nil {
		return err
	}
	if info.MSI.ProductCode != "" && !looksLikeGUID(info.MSI.ProductCode) {
		return fmt.Errorf("package msi.product_code %q must be a braced GUID", info.MSI.ProductCode)
	}
	if info.MSI.UpgradeCode != "" && !looksLikeGUID(info.MSI.UpgradeCode) {
		return fmt.Errorf("package msi.upgrade_code %q must be a braced GUID", info.MSI.UpgradeCode)
	}
	for k, v := range info.MSI.Properties {
		if v == "" {
			return fmt.Errorf("package msi.properties[%q] must not be empty", k)
		}
		if field, reserved := reservedProperties[k]; reserved {
			if field == "" {
				return fmt.Errorf("package msi.properties[%q] is set by nfpm and cannot be overridden", k)
			}
			return fmt.Errorf("package msi.properties[%q] is set by nfpm; use %s instead", k, field)
		}
	}

	dests, err := validateDestinations(info)
	if err != nil {
		return err
	}

	for i, s := range info.MSI.Shortcuts {
		if s.Name == "" {
			return fmt.Errorf("package msi.shortcuts[%d].name must be provided", i)
		}
		if s.Target == "" {
			return fmt.Errorf("package msi.shortcuts[%d].target must be provided", i)
		}
		if !dests[normalizeDest(s.Target)] {
			return fmt.Errorf("package msi.shortcuts[%d].target %q does not match any contents destination", i, s.Target)
		}
		if !shortcutDirectories[s.Directory] {
			return fmt.Errorf(
				"package msi.shortcuts[%d].directory %q is not a Windows Installer folder; "+
					"use INSTALLFOLDER or a standard folder such as ProgramMenuFolder, DesktopFolder, or StartupFolder",
				i, s.Directory)
		}
	}
	for i, s := range info.MSI.Services {
		if s.Name == "" {
			return fmt.Errorf("package msi.services[%d].name must be provided", i)
		}
		if s.Executable == "" {
			return fmt.Errorf("package msi.services[%d].executable must be provided", i)
		}
		if !dests[normalizeDest(s.Executable)] {
			return fmt.Errorf("package msi.services[%d].executable %q does not match any contents destination", i, s.Executable)
		}
		if _, ok := startTypes[strings.ToLower(s.StartType)]; !ok {
			return fmt.Errorf("package msi.services[%d].start_type %q is invalid", i, s.StartType)
		}
		if info.MSI.PerUser {
			return fmt.Errorf("package msi.services[%d] %q: Windows services require a per-machine install; unset msi.per_user", i, s.Name)
		}
	}
	for i, r := range info.MSI.Registry {
		root := strings.ToUpper(r.Root)
		if _, ok := registryRoots[root]; !ok {
			return fmt.Errorf("package msi.registry[%d].root %q is invalid", i, r.Root)
		}
		if r.Key == "" {
			return fmt.Errorf("package msi.registry[%d].key must be provided", i)
		}
		if info.MSI.PerUser && root == "HKLM" {
			return fmt.Errorf("package msi.registry[%d]: HKLM requires a per-machine install; use HKCU or HKMU, or unset msi.per_user", i)
		}
	}

	return nil
}

// validateVersion rejects a version that Windows Installer cannot represent.
// Clamping it instead would silently give distinct releases the same
// ProductVersion (and the same derived ProductCode), so they could no longer
// upgrade each other.
func validateVersion(info *nfpm.Info) error {
	_, clamped := msiVersionClamped(info)
	if !clamped {
		return nil
	}
	if info.MSI.Version != "" {
		return fmt.Errorf(
			"package msi.version %q exceeds the Windows Installer ProductVersion limits "+
				"(major and minor at most 255, build at most 65535)",
			info.MSI.Version)
	}
	return fmt.Errorf(
		"package version %q exceeds the Windows Installer ProductVersion limits "+
			"(major and minor at most 255, build at most 65535); set msi.version to a "+
			"version within the limits, for example 2024.1.0 -> 24.1.0",
		info.Version)
}

// validateDestinations checks that every content file has a usable Windows
// destination and that no two contents land on the same path once drive
// letters and slashes are normalized (files.PrepareForPackager only
// deduplicates literal duplicates). Per-user packages are also kept out of
// the per-machine folders they could not write without elevation. It returns
// the set of normalized destinations for cross-referencing shortcuts and
// services.
func validateDestinations(info *nfpm.Info) (map[string]bool, error) {
	dests := map[string]bool{}
	firstByDest := map[string]string{}
	for _, c := range info.Contents {
		if c.Type == files.TypeDir || c.Type == files.TypeImplicitDir || c.Type == files.TypeSymlink {
			continue
		}
		dest := normalizeDest(c.Destination)
		if dest == "" {
			return nil, fmt.Errorf("package contents destination %q is empty once the drive letter is removed; it must name a file", c.Destination)
		}
		if prev, dup := firstByDest[dest]; dup {
			return nil, fmt.Errorf("package contents destinations %q and %q resolve to the same Windows path %q", prev, c.Destination, dest)
		}
		firstByDest[dest] = c.Destination
		dests[dest] = true

		if info.MSI.PerUser {
			if rootID, _, _ := mapDestination(dest, is64bit(info.Arch)); perMachineDirs[rootID] {
				return nil, fmt.Errorf(
					"package contents destination %q is a per-machine location, which a per-user install cannot write to; "+
						"install it under the product folder or unset msi.per_user",
					c.Destination)
			}
		}
	}
	return dests, nil
}

// idTable hands out MSI identifiers and refuses to reuse one for a different
// seed, so a hash collision surfaces as a build error instead of two contents
// silently sharing a directory, component, or file row.
type idTable struct {
	seeds map[string]string
}

func newIDTable() *idTable {
	return &idTable{seeds: map[string]string{}}
}

// id returns the identifier for seed under prefix, or an error when another
// seed already produced the same identifier.
func (t *idTable) id(prefix, seed string) (string, error) {
	id := makeID(prefix, seed)
	if prev, ok := t.seeds[id]; ok && prev != seed {
		return "", fmt.Errorf("msi identifier %s is derived from both %q and %q; rename one of them", id, prev, seed)
	}
	t.seeds[id] = seed
	return id, nil
}

// addContents maps every content file to a directory/component/file in the MSI,
// honoring well-known Windows destination prefixes. Returns a placement map
// keyed by normalized destination path.
func addContents(b msi.PackageBuilder, info *nfpm.Info, upgradeCode string, ids *idTable, createdDirs map[string]bool) (map[string]placement, error) {
	placed := map[string]placement{}
	licenseSet := false
	warnedPermanent := map[string]bool{}
	is64 := is64bit(info.Arch)

	for _, content := range info.Contents {
		switch content.Type {
		case files.TypeDir, files.TypeImplicitDir:
			// Directories are implicit in the MSI directory tree.
			continue
		case files.TypeSymlink:
			log.Printf("warning: msi does not support symlinks, skipping %s", content.Destination)
			continue
		case files.TypeConfig, files.TypeConfigNoReplace:
			log.Printf(
				"warning: msi has no configuration file handling, %s is installed as a regular file and replaced on upgrade",
				content.Destination)
		}
		if content.Source == "" {
			continue
		}

		// License contents are installed like any other file and additionally
		// feed the install UI license screen (first one wins).
		if content.Type == files.TypeRPMLicence || content.Type == files.TypeRPMLicense {
			if licenseSet {
				log.Printf("warning: multiple license contents, %s is not used for the install UI", content.Source)
			} else {
				text, err := os.ReadFile(content.Source)
				if err != nil {
					return nil, fmt.Errorf("reading license file %s: %w", content.Source, err)
				}
				b.WithLicenseText(string(text))
				licenseSet = true
			}
		}

		src, err := msi.FileSourceFromPath(content.Source)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", content.Source, err)
		}

		dest := normalizeDest(content.Destination)
		rootID, rootDefault, rel := mapDestination(dest, is64)

		// Ensure the root directory exists. Standard Windows Installer folders
		// (ProgramFiles64Folder, etc.) must be rooted at TARGETDIR; declaring
		// them as bare roots would leave them unparented and break source
		// resolution (error 2704). INSTALLFOLDER is pre-declared in Package.
		if !createdDirs[rootID] {
			b.Directory("TARGETDIR").Subdirectory(rootID, rootDefault)
			createdDirs[rootID] = true
		}
		if systemDirs[rootID] && !warnedPermanent[rootID] {
			log.Printf(
				"warning: msi: %s installs into a Windows system folder; Windows Installer never removes such files, "+
					"so they outlive every uninstall of the product",
				content.Destination)
			warnedPermanent[rootID] = true
		}

		segments := strings.Split(rel, "/")
		fileName := segments[len(segments)-1]
		dirSegments := segments[:len(segments)-1]

		// Build the subdirectory chain, caching by accumulated path.
		parentID := rootID
		accum := rootID
		for _, seg := range dirSegments {
			if seg == "" {
				continue
			}
			accum = accum + "/" + seg
			dirID, err := ids.id("d", accum)
			if err != nil {
				return nil, err
			}
			if !createdDirs[dirID] {
				b.Directory(parentID).Subdirectory(dirID, seg)
				createdDirs[dirID] = true
			}
			parentID = dirID
		}

		compID, err := ids.id("c", dest)
		if err != nil {
			return nil, err
		}
		// The component GUID is what Windows Installer uses to recognize the
		// same resource across releases, so it is derived from the identity
		// that survives releases (the upgrade code), the architecture, and the
		// destination; never from anything that changes per release.
		comp := b.Directory(parentID).Component(compID).
			WithGUID(deriveGUID("component|" + upgradeCode + "|" + info.Arch + "|" + dest)).
			WithAttributes(componentAttributes(rootID, is64))
		comp.WithFile(fileName, src)
		comp.AssociateToFeature(mainFeature)

		placed[dest] = placement{componentID: compID, rootID: rootID}
	}

	return placed, nil
}

func addShortcuts(b msi.PackageBuilder, info *nfpm.Info, placed map[string]placement, ids *idTable) error {
	for _, s := range info.MSI.Shortcuts {
		p, ok := placed[normalizeDest(s.Target)]
		if !ok {
			return fmt.Errorf("shortcut %q target %q was not installed", s.Name, s.Target)
		}
		sc := b.Directory(p.rootID).Component(p.componentID).
			Shortcut(s.Name, "").
			Advertised(mainFeature).
			InDirectory(s.Directory)
		if s.Description != "" {
			sc = sc.Description(s.Description)
		}
		if s.Arguments != "" {
			sc = sc.Arguments(s.Arguments)
		}
		if s.Icon != "" {
			iconSrc, err := msi.FileSourceFromPath(s.Icon)
			if err != nil {
				return fmt.Errorf("reading shortcut icon %s: %w", s.Icon, err)
			}
			iconID, err := ids.id("ico", s.Icon)
			if err != nil {
				return err
			}
			iconName := iconID + filepath.Ext(s.Icon)
			b.Icon(iconName, iconSrc)
			sc.Icon(iconName, 0)
		}
	}
	return nil
}

func addServices(b msi.PackageBuilder, info *nfpm.Info, placed map[string]placement) error {
	for _, s := range info.MSI.Services {
		p, ok := placed[normalizeDest(s.Executable)]
		if !ok {
			return fmt.Errorf("service %q executable %q was not installed", s.Name, s.Executable)
		}
		comp := b.Directory(p.rootID).Component(p.componentID)

		// The builder mutates in place and returns itself, so the return values
		// of the chained setters are intentionally not captured.
		si := comp.ServiceInstall(s.Name)
		si.WithType(msi.ServiceTypeOwnProcess)
		si.WithStartType(startTypes[strings.ToLower(s.StartType)])
		si.WithErrorControl(msi.ServiceErrorNormal)
		if s.DisplayName != "" {
			si.WithDisplayName(s.DisplayName)
		}
		if s.Description != "" {
			si.WithDescription(s.Description)
		}
		if s.Account != "" {
			si.WithStartName(s.Account)
		}
		if s.Arguments != "" {
			si.WithArguments(s.Arguments)
		}
		if len(s.Dependencies) > 0 {
			si.WithDependencies(s.Dependencies...)
		}

		// A service is always stopped before its files are installed or
		// replaced (so upgrades and repairs never hit a running executable),
		// and stopped and deleted on uninstall: without the delete event
		// Windows Installer leaves the registration behind, pointing at a file
		// it just removed.
		sc := comp.ServiceControl(s.Name)
		sc.OnInstall().Stop()
		if s.Start {
			sc.Start()
		}
		sc.OnUninstall().Stop().Delete()
	}
	return nil
}

func addRegistry(b msi.PackageBuilder, info *nfpm.Info, upgradeCode string, ids *idTable) error {
	for _, r := range info.MSI.Registry {
		root := strings.ToUpper(r.Root)
		identity := root + "|" + r.Key + "|" + r.Name
		compID, err := ids.id("reg", identity)
		if err != nil {
			return err
		}
		// Registry values are their own component, keyed by the value itself
		// (RegistryKeyPath attribute), with a GUID derived like file
		// components so it survives releases.
		attrs := componentAttributes(installFolder, is64bit(info.Arch)) | msidbComponentAttributesRegistryKeyPath
		comp := b.Directory(installFolder).Component(compID).
			WithGUID(deriveGUID("registry|" + upgradeCode + "|" + info.Arch + "|" + identity)).
			WithAttributes(attrs)
		comp.RegistryKey(registryRoots[root], r.Key).
			Value(r.Name, r.Value).
			AsKeyPath()
		comp.AssociateToFeature(mainFeature)
	}
	return nil
}

func configureSigning(b msi.PackageBuilder, info *nfpm.Info) error {
	pfxPath := info.MSI.Signature.PFXFile
	if _, err := os.Stat(pfxPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("PFX file not found: %w", err)
		}
		return fmt.Errorf("unable to access PFX file: %w", err)
	}

	sb := msi.NewSigner().WithPFX(pfxPath, info.MSI.Signature.KeyPassphrase)
	if info.MSI.Signature.TimestampURL != "" {
		sb = sb.WithTimestampURL(info.MSI.Signature.TimestampURL)
	}
	signer, err := sb.Build()
	if err != nil {
		return &nfpm.ErrSigningFailure{Err: fmt.Errorf("building signer: %w", err)}
	}

	b.WithSigner(signer)
	return nil
}

// startTypes are the start types the ServiceInstall table supports. The
// remaining Windows start types (boot and system) are driver-only and are
// rejected by CreateService for the Win32 own-process services emitted here.
// nolint: gochecknoglobals
var startTypes = map[string]msi.ServiceStartType{
	"auto":     msi.ServiceStartAuto,
	"demand":   msi.ServiceStartDemand,
	"disabled": msi.ServiceStartDisabled,
}

// nolint: gochecknoglobals
var registryRoots = map[string]msi.RegistryRoot{
	"HKLM": msi.RegistryRootHKLM,
	"HKCU": msi.RegistryRootHKCU,
	"HKCR": msi.RegistryRootHKCR,
	"HKMU": msi.RegistryRootHKMU,
	"HKU":  msi.RegistryRootHKU,
}

// shortcutDirectories are the directories a shortcut may be created in: the
// product's install folder and the standard Windows Installer folders that
// go-msi declares on demand.
// nolint: gochecknoglobals
var shortcutDirectories = map[string]bool{
	installFolder:          true,
	"ProgramMenuFolder":    true,
	"StartMenuFolder":      true,
	"StartupFolder":        true,
	"DesktopFolder":        true,
	"FavoritesFolder":      true,
	"SendToFolder":         true,
	"AppDataFolder":        true,
	"LocalAppDataFolder":   true,
	"CommonAppDataFolder":  true,
	"PersonalFolder":       true,
	"TemplateFolder":       true,
	"NetHoodFolder":        true,
	"PrintHoodFolder":      true,
	"RecentFolder":         true,
	"AdminToolsFolder":     true,
	"ProgramFilesFolder":   true,
	"ProgramFiles64Folder": true,
	"CommonFilesFolder":    true,
	"CommonFiles64Folder":  true,
	"WindowsFolder":        true,
	"SystemFolder":         true,
	"System64Folder":       true,
	"FontsFolder":          true,
	"TempFolder":           true,
	"WindowsVolume":        true,
}

// destPrefix maps a leading destination path (lowercased, slash-separated) to a
// well-known MSI directory ID. Longer prefixes are matched first.
// nolint: gochecknoglobals
var destPrefixes = []struct {
	prefix string
	dir64  string
	dir32  string
}{
	{"program files (x86)", "ProgramFilesFolder", "ProgramFilesFolder"},
	{"program files", "ProgramFiles64Folder", "ProgramFilesFolder"},
	{"programdata", "CommonAppDataFolder", "CommonAppDataFolder"},
	{"windows/system32", "System64Folder", "SystemFolder"},
	{"windows/syswow64", "SystemFolder", "SystemFolder"},
	{"windows/fonts", "FontsFolder", "FontsFolder"},
	{"windows", "WindowsFolder", "WindowsFolder"},
	{"appdata/local", "LocalAppDataFolder", "LocalAppDataFolder"},
	{"appdata/roaming", "AppDataFolder", "AppDataFolder"},
}

// systemDirs are directories whose components are marked Permanent to satisfy
// ICE09. Windows Installer never removes permanent components, so files
// installed there outlive the product.
// nolint: gochecknoglobals
var systemDirs = map[string]bool{
	"SystemFolder":   true,
	"System64Folder": true,
	"WindowsFolder":  true,
	"FontsFolder":    true,
}

// perMachineDirs are the destination roots a per-user (unelevated) install
// cannot write to.
// nolint: gochecknoglobals
var perMachineDirs = map[string]bool{
	"ProgramFiles64Folder": true,
	"ProgramFilesFolder":   true,
	"CommonAppDataFolder":  true,
	"System64Folder":       true,
	"SystemFolder":         true,
	"WindowsFolder":        true,
	"FontsFolder":          true,
}

// dir32Roots are the standard folders that always resolve to the 32-bit
// location on 64-bit Windows (Program Files (x86), SysWOW64). A 64-bit
// component must not be installed directly into them (ICE80).
// nolint: gochecknoglobals
var dir32Roots = map[string]bool{
	"ProgramFilesFolder": true,
	"SystemFolder":       true,
}

const (
	msidbComponentAttributesRegistryKeyPath int16 = 0x4
	msidbComponentAttributesPermanent       int16 = 0x10
	msidbComponentAttributes64bit           int16 = 0x100
)

// componentAttributes builds a component's attribute bits from the root
// directory it installs under. The 64-bit bit follows the root's bitness
// rather than the package's: a 64-bit package still installs 32-bit content
// into Program Files (x86) and SysWOW64. Attributes are always set explicitly
// so go-msi never applies its own platform default on top.
func componentAttributes(rootID string, is64 bool) int16 {
	var attrs int16
	if is64 && !dir32Roots[rootID] {
		attrs |= msidbComponentAttributes64bit
	}
	if systemDirs[rootID] {
		attrs |= msidbComponentAttributesPermanent
	}
	return attrs
}

// normalizeDest converts a destination path to a forward-slash relative path
// with any drive letter and leading slashes removed.
func normalizeDest(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	// Remove leading slashes (nfpm normalizes absolute paths to start with "/",
	// e.g. "C:/x" becomes "/C:/x").
	p = strings.TrimLeft(p, "/")
	// Strip a leading drive letter (e.g. "C:").
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}
	p = strings.TrimLeft(p, "/")
	// Collapse duplicate slashes and dot segments. On Windows a bare drive
	// such as "C:" reaches here as "C:." (filepath.Clean's drive-relative
	// form), which must count as empty just like "C:" does elsewhere.
	p = path.Clean(p)
	if p == "." {
		return ""
	}
	return p
}

// mapDestination resolves a normalized destination to a root directory ID, its
// DefaultDir value, and the path relative to that root (always ending in the
// file name).
func mapDestination(dest string, is64 bool) (rootID, rootDefault, rel string) {
	lower := strings.ToLower(dest)
	for _, p := range destPrefixes {
		if lower == p.prefix || strings.HasPrefix(lower, p.prefix+"/") {
			id := p.dir32
			if is64 {
				id = p.dir64
			}
			rest := strings.TrimPrefix(dest[len(p.prefix):], "/")
			if rest == "" {
				rest = path.Base(dest)
			}
			// Standard folders use "." as their DefaultDir.
			return id, ".", rest
		}
	}
	// Fallback: install under INSTALLFOLDER using the full relative path.
	return installFolder, "", dest
}

// makeID builds a stable, MSI-valid identifier from a prefix and a seed string.
func makeID(prefix, seed string) string {
	h := fnv.New32a()
	_, _ = io.WriteString(h, seed)

	readable := sanitizeID(path.Base(strings.TrimRight(seed, "/")))
	if len(readable) > 40 {
		readable = readable[:40]
	}
	return fmt.Sprintf("%s_%s_%08x", prefix, readable, h.Sum32())
}

// sanitizeID replaces characters that are invalid in MSI identifiers.
func sanitizeID(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '.':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

// guidPattern matches a canonical braced GUID ({8-4-4-4-12} hexadecimal),
// accepting either letter case.
var guidPattern = regexp.MustCompile(
	`^\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}$`)

func looksLikeGUID(s string) bool {
	return guidPattern.MatchString(s)
}

// deriveGUID produces a stable, braced uppercase GUID (RFC 4122 v5 style) from
// the given seed. The same seed always yields the same GUID, keeping builds
// reproducible.
//
// The seed formats used with it are a compatibility contract: the upgrade code
// and component GUIDs of packages already installed on users' machines are
// derived from them, so changing a format orphans those installs.
func deriveGUID(seed string) string {
	h := sha1.Sum([]byte("nfpm-msi:" + seed))
	var b [16]byte
	copy(b[:], h[:16])
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	s := fmt.Sprintf("%X", b[:])
	return fmt.Sprintf("{%s-%s-%s-%s-%s}", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

// releaseIdentity is the version string that distinguishes one release from
// another: msi.version verbatim when set, otherwise the shared version with
// its release, pre-release, and metadata parts. It deliberately uses the
// original values rather than the normalized ProductVersion, so 1.2.3-rc1 and
// 1.2.3 are different releases even though Windows Installer sees both as
// 1.2.3.
func releaseIdentity(info *nfpm.Info) string {
	if info.MSI.Version != "" {
		return info.MSI.Version
	}
	return info.Version + "|" + info.Release + "|" + info.Prerelease + "|" + info.VersionMetadata
}

// deriveProductCode derives the default ProductCode. Its namespace includes
// the release identity, so every release gets a new ProductCode — a Windows
// Installer requirement for major upgrades to trigger.
func deriveProductCode(info *nfpm.Info) string {
	return deriveGUID("product|" + info.MSI.Manufacturer + "|" + info.MSI.ProductName + "|" + info.Arch + "|" + releaseIdentity(info))
}

// deriveUpgradeCode derives the default UpgradeCode. It excludes the version,
// so it stays stable across releases of the same product and the Upgrade table
// can find older installs. Architecture is included: each arch is its own
// product line (no cross-arch upgrades).
func deriveUpgradeCode(info *nfpm.Info) string {
	return deriveGUID("upgrade|" + info.MSI.Manufacturer + "|" + info.MSI.ProductName + "|" + info.Arch)
}

// msiVersionMaxima are the per-field maxima Windows Installer enforces on the
// ProductVersion property: Major.Minor.Build, capped at 255.255.65535. A value
// outside them makes the package uninstallable.
// nolint: gochecknoglobals
var msiVersionMaxima = [3]int{255, 255, 65535}

// convertToMSIVersionClamped converts a semver-style version to MSI's
// Major.Minor.Build format. Each field is numeric and clamped to the maximum
// Windows Installer accepts for it; clamped reports whether any field had to
// be clamped (validation turns that into an error).
func convertToMSIVersionClamped(version string) (converted string, clamped bool) {
	version = strings.TrimPrefix(version, "v")
	// Drop any pre-release / build metadata. This also strips the sign off a
	// negative field, so no field can end up below zero.
	if i := strings.IndexAny(version, "-+"); i >= 0 {
		version = version[:i]
	}

	parts := strings.SplitN(version, ".", 4)
	result := make([]string, 3)
	for i := range 3 {
		result[i] = "0"
		if i < len(parts) {
			if n, err := strconv.Atoi(parts[i]); err == nil {
				if n > msiVersionMaxima[i] {
					n = msiVersionMaxima[i]
					clamped = true
				}
				result[i] = strconv.Itoa(n)
			}
		}
	}
	return strings.Join(result, "."), clamped
}

// msiVersion returns the ProductVersion for the package: msi.version when set,
// otherwise the shared version, converted to MSI's Major.Minor.Build format.
func msiVersion(info *nfpm.Info) string {
	converted, _ := msiVersionClamped(info)
	return converted
}

// msiVersionClamped is msiVersion, additionally reporting whether the source
// version had to be clamped to fit the ProductVersion limits.
func msiVersionClamped(info *nfpm.Info) (converted string, clamped bool) {
	return convertToMSIVersionClamped(cmp.Or(info.MSI.Version, info.Version))
}
