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
	"regexp"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"go.digitalxero.dev/go-msi"
)

const packagerName = "msi"

// mainFeature is the single primary feature every installed component is
// associated with.
const mainFeature = "MainFeature"

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

func ensureValidArch(info *nfpm.Info) *nfpm.Info {
	if info.MSI.Arch != "" {
		info.Arch = info.MSI.Arch
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

// vendorOrMaintainer returns info.Vendor, falling back to the name part of
// info.Maintainer ("Jane Doe <jane@example.com>" -> "Jane Doe").
func vendorOrMaintainer(info *nfpm.Info) string {
	if info.Vendor != "" {
		return info.Vendor
	}
	m := info.Maintainer
	if i := strings.IndexByte(m, '<'); i >= 0 {
		m = m[:i]
	}
	return strings.TrimSpace(m)
}

// SetPackagerDefaults sets default values for MSI-specific fields.
func (*MSI) SetPackagerDefaults(info *nfpm.Info) {
	if info.MSI.ProductName == "" {
		info.MSI.ProductName = info.Name
	}
	if info.MSI.Manufacturer == "" {
		info.MSI.Manufacturer = vendorOrMaintainer(info)
	}
	if info.MSI.InstallDir == "" {
		info.MSI.InstallDir = info.MSI.ProductName
	}
	if info.MSI.AllUsers == nil {
		allUsers := true
		info.MSI.AllUsers = &allUsers
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

	version, clamped := msiVersionClamped(info)
	if clamped {
		log.Printf(
			"warning: msi: version %q exceeds the Windows Installer ProductVersion limits "+
				"(major and minor at most 255, build at most 65535) and was clamped to %q; "+
				"set msi.version to choose the version the package declares",
			cmp.Or(info.MSI.Version, info.Version), version)
	}

	// validate rejects an architecture with no Windows Installer platform, so
	// the lookup cannot fail here.
	platform, _ := platformFor(info.Arch)

	b := msi.NewPackage().
		WithProductName(info.MSI.ProductName).
		WithManufacturer(info.MSI.Manufacturer).
		WithVersion(version).
		WithPlatform(platform).
		WithAllUsers(*info.MSI.AllUsers)

	// ProductCode must always be present. Windows Installer requires a new
	// ProductCode for every release for major upgrades to work, so the derived
	// default is namespaced by manufacturer, product name, architecture, and
	// the version as MSI compares it.
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

	// Declare INSTALLFOLDER explicitly so its DefaultDir is the configured
	// install directory name (otherwise go-msi derives it from the product name).
	b.RootDirectory("INSTALLFOLDER", info.MSI.InstallDir)

	// The single primary feature every component is associated with.
	b.Feature(mainFeature).WithTitle(info.MSI.ProductName).WithLevel(1)

	placed, err := addContents(b, info)
	if err != nil {
		return err
	}

	if err := addShortcuts(b, info, placed); err != nil {
		return err
	}
	if err := addServices(b, info, placed); err != nil {
		return err
	}
	addRegistry(b, info)

	if info.MSI.Upgrade.Enabled {
		mu := b.MajorUpgrade()
		if info.MSI.Upgrade.DowngradeErrorMessage != "" {
			mu.DowngradeErrorMessage(info.MSI.Upgrade.DowngradeErrorMessage)
		}
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

// placement records where a content file was installed so shortcuts and services
// can reference it by its original destination path.
type placement struct {
	componentID string
	rootID      string
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
	if err := validateScripts(info); err != nil {
		return err
	}
	if info.MSI.ProductCode != "" && !looksLikeGUID(info.MSI.ProductCode) {
		return fmt.Errorf("package msi.product_code %q must be a braced GUID", info.MSI.ProductCode)
	}
	if info.MSI.UpgradeCode != "" && !looksLikeGUID(info.MSI.UpgradeCode) {
		return fmt.Errorf("package msi.upgrade_code %q must be a braced GUID", info.MSI.UpgradeCode)
	}

	dests := map[string]bool{}
	for _, c := range info.Contents {
		if c.Type == files.TypeDir || c.Type == files.TypeImplicitDir || c.Type == files.TypeSymlink {
			continue
		}
		dests[normalizeDest(c.Destination)] = true
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
	}
	for i, r := range info.MSI.Registry {
		if _, ok := registryRoots[strings.ToUpper(r.Root)]; !ok {
			return fmt.Errorf("package msi.registry[%d].root %q is invalid", i, r.Root)
		}
		if r.Key == "" {
			return fmt.Errorf("package msi.registry[%d].key must be provided", i)
		}
	}

	return nil
}

// addContents maps every content file to a directory/component/file in the MSI,
// honoring well-known Windows destination prefixes. Returns a placement map
// keyed by normalized destination path.
func addContents(b msi.PackageBuilder, info *nfpm.Info) (map[string]placement, error) {
	placed := map[string]placement{}
	createdDirs := map[string]bool{"INSTALLFOLDER": true}
	licenseSet := false

	for _, content := range info.Contents {
		switch content.Type {
		case files.TypeDir, files.TypeImplicitDir:
			// Directories are implicit in the MSI directory tree.
			continue
		case files.TypeSymlink:
			log.Printf("warning: msi does not support symlinks, skipping %s", content.Destination)
			continue
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
		rootID, rootDefault, rel := mapDestination(dest, is64bit(info.Arch))

		// Ensure the root directory exists. Standard Windows Installer folders
		// (ProgramFiles64Folder, etc.) must be rooted at TARGETDIR; declaring
		// them as bare roots would leave them unparented and break source
		// resolution (error 2704). INSTALLFOLDER is pre-declared in Package and
		// reparented by go-msi.
		if !createdDirs[rootID] {
			b.Directory("TARGETDIR").Subdirectory(rootID, rootDefault)
			createdDirs[rootID] = true
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
			dirID := makeID("d", accum)
			if !createdDirs[dirID] {
				b.Directory(parentID).Subdirectory(dirID, seg)
				createdDirs[dirID] = true
			}
			parentID = dirID
		}

		compID := makeID("c", dest)
		comp := b.Directory(parentID).Component(compID).WithGUID("")
		if attrs := componentAttributes(rootID, is64bit(info.Arch)); attrs != 0 {
			comp = comp.WithAttributes(attrs)
		}
		comp.WithFile(fileName, src)
		comp.AssociateToFeature(mainFeature)

		placed[dest] = placement{componentID: compID, rootID: rootID}
	}

	return placed, nil
}

func addShortcuts(b msi.PackageBuilder, info *nfpm.Info, placed map[string]placement) error {
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
			iconName := makeID("ico", s.Icon) + path.Ext(s.Icon)
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

		if s.Start || s.Stop {
			sc := comp.ServiceControl(s.Name)
			if s.Start {
				sc.OnInstall().Start()
			}
			if s.Stop {
				sc.OnUninstall().Stop().Delete()
			}
		}
	}
	return nil
}

func addRegistry(b msi.PackageBuilder, info *nfpm.Info) {
	for i, r := range info.MSI.Registry {
		compID := makeID("reg", strconv.Itoa(i)+"|"+r.Root+"|"+r.Key+"|"+r.Name)
		comp := b.Directory("INSTALLFOLDER").Component(compID).WithGUID("")
		comp.RegistryKey(registryRoots[strings.ToUpper(r.Root)], r.Key).
			Value(r.Name, r.Value).
			AsKeyPath()
		comp.AssociateToFeature(mainFeature)
	}
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

// systemDirs are directories whose components should be marked Permanent to
// satisfy ICE09.
// nolint: gochecknoglobals
var systemDirs = map[string]bool{
	"SystemFolder":   true,
	"System64Folder": true,
	"WindowsFolder":  true,
	"FontsFolder":    true,
}

const (
	msidbComponentAttributes64bit     int16 = 0x100
	msidbComponentAttributesPermanent int16 = 0x10
)

// componentAttributes builds a component's attribute bits. The 64-bit bit is
// set here rather than left to go-msi: go-msi only applies it to components
// whose attributes the caller never set, so a component that needs Permanent
// would otherwise lose it. Components with no attributes at all (registry
// components, for instance) are left alone and go-msi handles them.
func componentAttributes(rootID string, is64 bool) int16 {
	var attrs int16
	if is64 {
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
	// Collapse any duplicate slashes.
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
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
	return "INSTALLFOLDER", "", dest
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
func deriveGUID(seed string) string {
	h := sha1.Sum([]byte("nfpm-msi:" + seed))
	var b [16]byte
	copy(b[:], h[:16])
	b[6] = (b[6] & 0x0f) | 0x50 // version 5
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	s := fmt.Sprintf("%X", b[:])
	return fmt.Sprintf("{%s-%s-%s-%s-%s}", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}

// deriveProductCode derives the default ProductCode. Its namespace includes
// the version (as MSI compares it), so every release gets a new ProductCode —
// a Windows Installer requirement for major upgrades to trigger.
func deriveProductCode(info *nfpm.Info) string {
	return deriveGUID("product|" + info.MSI.Manufacturer + "|" + info.MSI.ProductName + "|" + info.Arch + "|" + msiVersion(info))
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

// convertToMSIVersion converts a semver-style version to MSI's
// Major.Minor.Build format. Each field is numeric and clamped to the maximum
// Windows Installer accepts for it.
func convertToMSIVersion(version string) string {
	converted, _ := convertToMSIVersionClamped(version)
	return converted
}

// convertToMSIVersionClamped is convertToMSIVersion, additionally reporting
// whether any field had to be clamped to stay within the ProductVersion limits.
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
	if info.MSI.Version != "" {
		return convertToMSIVersionClamped(info.MSI.Version)
	}
	return convertToMSIVersionClamped(info.Version)
}
