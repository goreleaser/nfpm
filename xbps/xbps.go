// Package xbps implements nfpm.Packager providing .xbps bindings.
package xbps

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
	"github.com/klauspost/compress/zstd"
)

const packagerName = "xbps"

// nolint: gochecknoinits
func init() {
	nfpm.RegisterPackager(packagerName, Default)
}

// archToXBPS maps generic and native architecture names to the XBPS name.
// Every XBPS name maps to itself so that normalization is idempotent: Package
// normalizes info.Arch once, and later lookups of the already-normalized value
// (propsManifest, ConventionalFileName) must succeed.
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
	"armv6l":  "armv6l",
	"arm7":    "armv7l",
	"armv7l":  "armv7l",
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

// normalizeVersionPart makes one version component safe for use inside an
// XBPS pkgver. XBPS parses "pkgname-version_revision" from the right
// (xbps_pkg_name and xbps_pkg_version in lib/util.c): the last '-' separates
// the package name from the version and the first '_' after a digit separates
// the version from the revision. A '-' or '_' anywhere inside the version would
// therefore move the split point and change the package name xbps-rindex
// indexes, so both are folded to '.', which XBPS treats as an ordinary version
// separator. Repeated and edge dots are collapsed so the result is stable.
func normalizeVersionPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch r {
		case '-', '_':
			return '.'
		default:
			return r
		}
	}, value)
	for strings.Contains(value, "..") {
		value = strings.ReplaceAll(value, "..", ".")
	}
	return strings.Trim(value, ".")
}

func version(info *nfpm.Info) string {
	base := strings.TrimPrefix(strings.TrimSpace(info.Version), "v")

	parts := make([]string, 0, 3)
	for _, part := range []string{base, info.Prerelease, info.VersionMetadata} {
		if normalized := normalizeVersionPart(part); normalized != "" {
			parts = append(parts, normalized)
		}
	}
	return strings.Join(parts, ".")
}

// isRevision mirrors is_revision() in xbps lib/util.c: digits and underscores
// only, non-empty.
func isRevision(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// splitPkgver mirrors xbps_pkg_name() and xbps_pkg_version() in xbps
// lib/util.c. The package name is everything before the last '-'. Scanning the
// remainder, an '_' before any digit makes the pkgver invalid; the first digit
// must be followed (not necessarily immediately) by an '_' whose remainder is a
// revision. It is the contract every pkgver produced here must satisfy, since
// xbps-rindex, xbps-install and xbps-query all resolve names through it.
func splitPkgver(pv string) (name, ver, rev string, ok bool) {
	hyphen := strings.LastIndexByte(pv, '-')
	if hyphen < 0 {
		return "", "", "", false
	}
	rest := pv[hyphen+1:]
	for i := 0; i < len(rest); i++ {
		switch c := rest[i]; {
		case c == '_':
			return "", "", "", false
		case c >= '0' && c <= '9':
			underscore := strings.IndexByte(rest[i+1:], '_')
			if underscore < 0 {
				return "", "", "", false
			}
			underscore += i + 1
			if !isRevision(rest[underscore+1:]) {
				return "", "", "", false
			}
			return pv[:hyphen], rest[:underscore], rest[underscore+1:], true
		}
	}
	return "", "", "", false
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
	ver := version(info)
	pv := fmt.Sprintf("%s-%s_%s", info.Name, ver, rev)

	// Round-trip through the XBPS parser so a pkgver that XBPS would index
	// under a different name or version is rejected at build time instead of
	// producing a package that cannot be installed or upgraded by name.
	name, parsedVer, parsedRev, ok := splitPkgver(pv)
	if !ok || name != info.Name || parsedVer != ver || parsedRev != rev {
		return "", fmt.Errorf("xbps: pkgver %q is not parseable by xbps: the version %q must contain a digit and the package name must not be empty", pv, ver)
	}
	return pv, nil
}

func shortDesc(info *nfpm.Info) string {
	if desc := strings.TrimSpace(info.XBPS.ShortDesc); desc != "" {
		return desc
	}
	first, _, _ := strings.Cut(strings.TrimSpace(info.Description), "\n")
	return strings.TrimSpace(first)
}

func sortedContents(info *nfpm.Info) files.Contents {
	contents := slices.Clone(info.Contents)
	sort.Sort(contents)
	return contents
}

func sortedStrings(values []string) []string {
	result := slices.Clone(values)
	sort.Strings(result)
	return result
}

func isConfigType(contentType string) bool {
	switch contentType {
	case files.TypeConfig, files.TypeConfigNoReplace, files.TypeConfigMissingOK:
		return true
	default:
		return false
	}
}

func isPayloadFileType(contentType string) bool {
	switch contentType {
	case files.TypeDir, files.TypeImplicitDir, files.TypeSymlink, files.TypeRPMGhost:
		return false
	default:
		return true
	}
}

func installedSize(info *nfpm.Info) uint64 {
	var total uint64
	for _, content := range info.Contents {
		if isPayloadFileType(content.Type) {
			total += uint64(content.Size())
		}
	}
	return total
}

func configFiles(info *nfpm.Info) []string {
	var result []string
	for _, content := range info.Contents {
		if isConfigType(content.Type) {
			result = append(result, files.NormalizeAbsoluteFilePath(content.Destination))
		}
	}
	return result
}

// explicitDirs returns the directories the user listed with type "dir",
// sorted by destination. XBPS's unpacker ignores directory entries in the
// archive (lib/package_unpack.c), so these are created by the generated
// INSTALL script instead, mirroring the xbps-src make_dirs mechanism.
// Implicit parent directories are excluded: extraction of the files inside
// them creates them, exactly as it does for native packages.
func explicitDirs(info *nfpm.Info) []*files.Content {
	var dirs []*files.Content
	for _, content := range info.Contents {
		if content.Type == files.TypeDir {
			dirs = append(dirs, content)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Destination < dirs[j].Destination
	})
	return dirs
}

func stringsToPlistArray(values []string) plistArray {
	items := plistArray{}
	for _, value := range sortedStrings(values) {
		items = append(items, value)
	}
	return items
}

// patternChars are the characters xbps_pkgpattern_name() (lib/util.c) treats
// as the start of a version constraint in a dependency.
const patternChars = "><*?[]"

// runDependency converts one generic nFPM dependency into a form XBPS's
// dependency resolver accepts (lib/transaction_pkg_deps.c). XBPS requires
// either a pattern ("bash>=5.0_1") or an exact pkgver ("bash-5.2_1"); a bare
// package name such as "bash" fails with "can't guess pkgname for dependency",
// so bare names become the unconstrained pattern "bash>=0". Native forms pass
// through unchanged. Syntax from other packagers (spaces, parentheses) is
// rejected so it cannot be mis-parsed as part of a package name.
func runDependency(dep string) (string, error) {
	dep = strings.TrimSpace(dep)
	if dep == "" {
		return "", fmt.Errorf("xbps: empty dependency")
	}
	if strings.ContainsAny(dep, " \t\r\n()") {
		return "", fmt.Errorf("xbps: dependency %q must use xbps syntax: a package name, name-version_revision, or name followed by a version pattern such as >=1.0", dep)
	}
	if idx := strings.IndexAny(dep, patternChars); idx >= 0 {
		// Mirror xbps_pkgpattern_name: the name is the prefix before the
		// pattern, minus one trailing '-'.
		if name := strings.TrimSuffix(dep[:idx], "-"); name == "" {
			return "", fmt.Errorf("xbps: dependency %q has a version pattern but no package name", dep)
		}
		return dep, nil
	}
	if _, _, _, ok := splitPkgver(dep); ok {
		return dep, nil
	}
	return dep + ">=0", nil
}

func runDepends(depends []string) ([]string, error) {
	result := make([]string, 0, len(depends))
	for _, dep := range depends {
		converted, err := runDependency(dep)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func invalidAlternativePart(value string) bool {
	return value == "" || strings.ContainsAny(value, ": \t\n\r")
}

func validateAlternative(alt nfpm.XBPSAlternative) error {
	switch {
	case invalidAlternativePart(alt.Group):
		return fmt.Errorf("xbps: invalid alternative group %q", alt.Group)
	case invalidAlternativePart(alt.LinkName):
		return fmt.Errorf("xbps: invalid alternative link_name %q", alt.LinkName)
	case invalidAlternativePart(alt.Target):
		return fmt.Errorf("xbps: invalid alternative target %q", alt.Target)
	default:
		return nil
	}
}

func sortedAlternatives(values []nfpm.XBPSAlternative) []nfpm.XBPSAlternative {
	result := slices.Clone(values)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		if result[i].LinkName != result[j].LinkName {
			return result[i].LinkName < result[j].LinkName
		}
		return result[i].Target < result[j].Target
	})
	return result
}

func alternativesMetadata(info *nfpm.Info) (plistDict, error) {
	result := plistDict{}
	for _, alt := range sortedAlternatives(info.XBPS.Alternatives) {
		if err := validateAlternative(alt); err != nil {
			return nil, err
		}
		item := fmt.Sprintf("%s:%s", alt.LinkName, alt.Target)
		items, _ := result[alt.Group].(plistArray)
		result[alt.Group] = append(items, item)
	}
	return result, nil
}

type plistValue any

type plistDict map[string]plistValue

type plistArray []plistValue

func portablePlistEscape(buf *bytes.Buffer, value string) {
	for _, r := range value {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
}

func writePlistValue(buf *bytes.Buffer, value plistValue) error {
	switch v := value.(type) {
	case plistDict:
		buf.WriteString("<dict>")
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			buf.WriteString("<key>")
			portablePlistEscape(buf, key)
			buf.WriteString("</key>")
			if err := writePlistValue(buf, v[key]); err != nil {
				return err
			}
		}
		buf.WriteString("</dict>")
	case plistArray:
		buf.WriteString("<array>")
		for _, item := range v {
			if err := writePlistValue(buf, item); err != nil {
				return err
			}
		}
		buf.WriteString("</array>")
	case string:
		buf.WriteString("<string>")
		portablePlistEscape(buf, v)
		buf.WriteString("</string>")
	case bool:
		if v {
			buf.WriteString("<true/>")
		} else {
			buf.WriteString("<false/>")
		}
	case int64:
		fmt.Fprintf(buf, "<integer>%d</integer>", v)
	case uint64:
		fmt.Fprintf(buf, "<integer>%d</integer>", v)
	default:
		return fmt.Errorf("xbps: unsupported plist value type %T", value)
	}
	return nil
}

func marshalPlist(root plistDict) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	buf.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple Computer//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	buf.WriteString("<plist version=\"1.0\">\n")
	if err := writePlistValue(&buf, root); err != nil {
		return nil, err
	}
	buf.WriteString("\n</plist>\n")
	return buf.Bytes(), nil
}

func propsManifest(info *nfpm.Info) (plistDict, error) {
	copyInfo := *info
	normalized, err := ensureValidArch(&copyInfo)
	if err != nil {
		return nil, err
	}
	pv, err := pkgver(info)
	if err != nil {
		return nil, err
	}

	manifest := plistDict{
		"architecture":   normalized.Arch,
		"installed_size": installedSize(info),
		"pkgname":        info.Name,
		"pkgver":         pv,
		"short_desc":     shortDesc(info),
		"version":        version(info),
	}
	if info.Description != "" {
		manifest["long_desc"] = info.Description
	}
	if info.Homepage != "" {
		manifest["homepage"] = info.Homepage
	}
	if info.License != "" {
		manifest["license"] = info.License
	}
	if info.Maintainer != "" {
		manifest["maintainer"] = info.Maintainer
	}
	if len(info.Depends) > 0 {
		depends, err := runDepends(info.Depends)
		if err != nil {
			return nil, err
		}
		manifest["run_depends"] = stringsToPlistArray(depends)
	}
	if confs := configFiles(info); len(confs) > 0 {
		manifest["conf_files"] = stringsToPlistArray(confs)
	}
	for key, values := range map[string][]string{
		"conflicts": info.Conflicts,
		"provides":  info.Provides,
		"replaces":  info.Replaces,
		"reverts":   info.XBPS.Reverts,
	} {
		if len(values) == 0 {
			continue
		}
		manifest[key] = stringsToPlistArray(values)
	}
	if len(info.XBPS.Tags) > 0 {
		manifest["tags"] = strings.Join(sortedStrings(info.XBPS.Tags), " ")
	}
	if info.XBPS.Preserve {
		manifest["preserve"] = true
	}
	if len(info.XBPS.Alternatives) > 0 {
		alternatives, err := alternativesMetadata(info)
		if err != nil {
			return nil, err
		}
		manifest["alternatives"] = alternatives
	}
	return manifest, nil
}

func fileEntry(content *files.Content) (plistDict, error) {
	entry := plistDict{
		"file": files.NormalizeAbsoluteFilePath(content.Destination),
	}
	if content.Type == files.TypeSymlink {
		// Record the symlink target exactly as it is written into the tar
		// header (writeContentEntry sets Linkname: content.Source), so the
		// files.plist metadata never disagrees with the payload for relative
		// targets. This matches the arch packager's MTREE handling.
		entry["target"] = content.Source
		return entry, nil
	}
	if content.Type == files.TypeDir || content.Type == files.TypeImplicitDir {
		return entry, nil
	}

	f, err := os.Open(content.Source)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	entry["sha256"] = fmt.Sprintf("%x", h.Sum(nil))
	entry["size"] = uint64(content.Size())
	return entry, nil
}

func filesManifest(info *nfpm.Info) (plistDict, error) {
	manifest := plistDict{}
	var regular plistArray
	var configs plistArray
	var links plistArray
	var dirs plistArray

	for _, content := range sortedContents(info) {
		if content.Type == files.TypeRPMGhost {
			continue
		}
		entry, err := fileEntry(content)
		if err != nil {
			return nil, err
		}
		switch {
		case content.Type == files.TypeDir || content.Type == files.TypeImplicitDir:
			dirs = append(dirs, entry)
		case content.Type == files.TypeSymlink:
			links = append(links, entry)
		case isConfigType(content.Type):
			configs = append(configs, entry)
		default:
			regular = append(regular, entry)
		}
	}
	if len(regular) > 0 {
		manifest["files"] = regular
	}
	if len(configs) > 0 {
		manifest["conf_files"] = configs
	}
	if len(links) > 0 {
		manifest["links"] = links
	}
	if len(dirs) > 0 {
		manifest["dirs"] = dirs
	}
	return manifest, nil
}

// xbpsShellFunction is one shell function emitted into a generated INSTALL or
// REMOVE script.
type xbpsShellFunction struct {
	name string
	body []byte
}

// xbpsActionCase maps one XBPS ACTION ($1, "pre" or "post") to the shell
// statements the generated script runs for it, in order.
type xbpsActionCase struct {
	action     string
	statements []string
}

func loadOptionalScript(source string) ([]byte, error) {
	if source == "" {
		return nil, nil
	}
	return os.ReadFile(source)
}

func appendScriptFunction(buf *bytes.Buffer, fn xbpsShellFunction) {
	fmt.Fprintf(buf, "%s() {\n", fn.name)
	buf.Write(fn.body)
	if len(fn.body) > 0 && fn.body[len(fn.body)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n\n")
}

// shellQuote returns value as a single-quoted POSIX shell word.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// renderXBPSActionScript emits the INSTALL/REMOVE wrapper. XBPS runs these
// scripts as
//
//	sh SCRIPT ACTION PKGNAME VERSION UPDATE CONF ARCH
//
// with ACTION being "pre" or "post", UPDATE "yes" or "no", and the package
// root directory as the working directory (lib/package_script.c). Every user
// function is invoked with "$@" so hooks see the full native argument list,
// in particular the update flag in $4.
func renderXBPSActionScript(functions []xbpsShellFunction, cases []xbpsActionCase) []byte {
	if len(cases) == 0 {
		return nil
	}
	var buf bytes.Buffer
	buf.WriteString("#!/bin/sh\n")
	buf.WriteString("# Generated by nFPM. XBPS invokes this script as:\n")
	buf.WriteString("#   sh SCRIPT ACTION PKGNAME VERSION UPDATE CONF ARCH\n")
	buf.WriteString("# with the package root directory as the working directory.\n\n")
	for _, fn := range functions {
		appendScriptFunction(&buf, fn)
	}

	buf.WriteString("case \"$1\" in\n")
	for _, actionCase := range cases {
		fmt.Fprintf(&buf, "%s)\n", actionCase.action)
		for _, statement := range actionCase.statements {
			fmt.Fprintf(&buf, "\t%s\n", statement)
		}
		buf.WriteString("\t;;\n")
	}
	buf.WriteString("esac\n")
	return buf.Bytes()
}

// dirMode renders the directory mode for chmod. Go's os.FileMode keeps
// setuid/setgid/sticky in high bits (os.ModeSetuid and friends), while a
// YAML-parsed octal such as 02775 keeps them at the traditional positions
// (0o4000, 0o2000, 0o1000); both encodings reach this point, so both are
// honoured, as the rpm packager does.
func dirMode(content *files.Content) string {
	mode := content.Mode()
	bits := uint32(mode.Perm())
	if mode&os.ModeSetuid != 0 || uint32(mode)&0o4000 != 0 {
		bits |= 0o4000
	}
	if mode&os.ModeSetgid != 0 || uint32(mode)&0o2000 != 0 {
		bits |= 0o2000
	}
	if mode&os.ModeSticky != 0 || uint32(mode)&0o1000 != 0 {
		bits |= 0o1000
	}
	return fmt.Sprintf("%04o", bits)
}

func dirOwner(value string) string {
	if value == "" {
		return "root"
	}
	return value
}

// makeDirsFunctions generates the shell that creates explicit directories at
// post-install and removes them at post-remove. It mirrors the xbps-triggers
// "mkdirs" trigger that xbps-src uses for make_dirs: mkdir -p, then chown and
// chmod, with failures reported but not fatal; removal uses rmdir and ignores
// non-empty or missing directories. Paths are made relative to the working
// directory, which XBPS sets to the package root.
func makeDirsFunctions(dirs []*files.Content) (create []xbpsShellFunction, remove xbpsShellFunction) {
	var createBody, removeBody bytes.Buffer
	for _, dir := range dirs {
		path := shellQuote(files.NormalizeAbsoluteFilePath(dir.Destination))
		fmt.Fprintf(&createBody, "\tnfpm_make_dir %s %s %s %s\n",
			path, dirMode(dir), shellQuote(dirOwner(dir.FileInfo.Owner)), shellQuote(dirOwner(dir.FileInfo.Group)))
		// Written so that a non-empty directory never fails the script, even
		// if a user hook enabled "set -e" in the same shell.
		fmt.Fprintf(&removeBody, "\tif [ -d .%s ]; then rmdir .%s 2>/dev/null || :; fi\n", path, path)
	}
	removeBody.WriteString("\t:\n")

	create = []xbpsShellFunction{
		{
			name: "nfpm_make_dir",
			body: []byte(`	# $1 path, $2 mode, $3 owner, $4 group
	if [ ! -d ".$1" ]; then
		mkdir -p ".$1" || echo "nfpm: failed to create directory $1" >&2
	fi
	chown "$3:$4" ".$1" 2>/dev/null || echo "nfpm: failed to set owner of $1 to $3:$4" >&2
	chmod "$2" ".$1" || echo "nfpm: failed to set mode of $1 to $2" >&2
`),
		},
		{name: "nfpm_make_dirs", body: createBody.Bytes()},
	}
	remove = xbpsShellFunction{name: "nfpm_remove_dirs", body: removeBody.Bytes()}
	return create, remove
}

func renderInstallScript(info *nfpm.Info) ([]byte, error) {
	pre, err := loadOptionalScript(info.Scripts.PreInstall)
	if err != nil {
		return nil, err
	}
	post, err := loadOptionalScript(info.Scripts.PostInstall)
	if err != nil {
		return nil, err
	}

	var functions []xbpsShellFunction
	var preStatements, postStatements []string
	if len(pre) > 0 {
		functions = append(functions, xbpsShellFunction{name: "preinstall", body: pre})
		preStatements = append(preStatements, `preinstall "$@"`)
	}
	if dirs := explicitDirs(info); len(dirs) > 0 {
		create, _ := makeDirsFunctions(dirs)
		functions = append(functions, create...)
		postStatements = append(postStatements, "nfpm_make_dirs")
	}
	if len(post) > 0 {
		functions = append(functions, xbpsShellFunction{name: "postinstall", body: post})
		postStatements = append(postStatements, `postinstall "$@"`)
	}

	var cases []xbpsActionCase
	if len(preStatements) > 0 {
		cases = append(cases, xbpsActionCase{action: "pre", statements: preStatements})
	}
	if len(postStatements) > 0 {
		cases = append(cases, xbpsActionCase{action: "post", statements: postStatements})
	}
	return renderXBPSActionScript(functions, cases), nil
}

func renderRemoveScript(info *nfpm.Info) ([]byte, error) {
	pre, err := loadOptionalScript(info.Scripts.PreRemove)
	if err != nil {
		return nil, err
	}
	post, err := loadOptionalScript(info.Scripts.PostRemove)
	if err != nil {
		return nil, err
	}

	var functions []xbpsShellFunction
	var preStatements, postStatements []string
	if len(pre) > 0 {
		functions = append(functions, xbpsShellFunction{name: "preremove", body: pre})
		preStatements = append(preStatements, `preremove "$@"`)
	}
	if len(post) > 0 {
		functions = append(functions, xbpsShellFunction{name: "postremove", body: post})
		postStatements = append(postStatements, `postremove "$@"`)
	}
	if dirs := explicitDirs(info); len(dirs) > 0 {
		_, remove := makeDirsFunctions(dirs)
		functions = append(functions, remove)
		postStatements = append(postStatements, "nfpm_remove_dirs")
	}

	var cases []xbpsActionCase
	if len(preStatements) > 0 {
		cases = append(cases, xbpsActionCase{action: "pre", statements: preStatements})
	}
	if len(postStatements) > 0 {
		cases = append(cases, xbpsActionCase{action: "post", statements: postStatements})
	}
	return renderXBPSActionScript(functions, cases), nil
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

func writeBytesEntry(tw *tar.Writer, name string, data []byte, mode int64, info *nfpm.Info) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
		ModTime:  info.MTime,
		Uname:    "root",
		Gname:    "root",
		Uid:      0,
		Gid:      0,
	}); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func packageShouldBeSigned(info *nfpm.Info) bool {
	return info.XBPS.Signature.KeyFile != "" || info.XBPS.Signature.SignFn != nil
}

func requireSignatureTarget(info *nfpm.Info) error {
	if strings.TrimSpace(info.Target) == "" {
		return &nfpm.ErrSigningFailure{Err: fmt.Errorf("xbps: target path required for signature sidecar")}
	}
	return nil
}

func signPackageDigest(info *nfpm.Info, digest []byte) ([]byte, error) {
	if info.XBPS.Signature.SignFn != nil {
		signature, err := info.XBPS.Signature.SignFn(bytes.NewReader(digest))
		if err != nil {
			return nil, fmt.Errorf("sign package digest: %w", err)
		}
		return signature, nil
	}
	return sign.RSASignSHA256Digest(digest, info.XBPS.Signature.KeyFile, info.XBPS.Signature.KeyPassphrase)
}

func sidecarPath(info *nfpm.Info) string {
	return info.Target + ".sig2"
}

// removeStaleSidecar deletes a signature sidecar left behind by a previous
// build of the same target. The CLI removes only the package file when a build
// fails (internal/cmd/package.go), so without this an unsigned or failed
// rebuild would leave a .sig2 that no longer matches the package bytes, and a
// signed repository would then reject the package during verification.
func removeStaleSidecar(info *nfpm.Info) error {
	if strings.TrimSpace(info.Target) == "" {
		return nil
	}
	if err := os.Remove(sidecarPath(info)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("xbps: remove stale signature sidecar: %w", err)
	}
	return nil
}

func writeSignatureSidecar(info *nfpm.Info, digest []byte) error {
	signature, err := signPackageDigest(info, digest)
	if err != nil {
		return &nfpm.ErrSigningFailure{Err: err}
	}
	if err := os.WriteFile(sidecarPath(info), signature, 0o644); err != nil {
		return &nfpm.ErrSigningFailure{Err: fmt.Errorf("write signature sidecar: %w", err)}
	}
	return nil
}

func writeContentEntry(tw *tar.Writer, content *files.Content) error {
	name := files.AsExplicitRelativePath(content.Destination)
	switch content.Type {
	case files.TypeDir, files.TypeImplicitDir:
		return tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     int64(content.Mode()),
			Typeflag: tar.TypeDir,
			ModTime:  content.ModTime(),
			Uname:    content.FileInfo.Owner,
			Gname:    content.FileInfo.Group,
			Uid:      0,
			Gid:      0,
		})
	case files.TypeSymlink:
		return tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o777,
			Typeflag: tar.TypeSymlink,
			Linkname: content.Source,
			ModTime:  content.ModTime(),
			Uname:    content.FileInfo.Owner,
			Gname:    content.FileInfo.Group,
			Uid:      0,
			Gid:      0,
		})
	default:
		f, err := os.Open(content.Source)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     int64(content.Mode()),
			Size:     content.Size(),
			Typeflag: tar.TypeReg,
			ModTime:  content.ModTime(),
			Uname:    content.FileInfo.Owner,
			Gname:    content.FileInfo.Group,
			Uid:      0,
			Gid:      0,
		}); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	}
}

// controlEntry is one of the metadata files at the head of the archive.
type controlEntry struct {
	name string
	data []byte
	mode int64
}

// controlEntries returns the metadata entries in the order XBPS requires.
// Both the unpacker (lib/package_unpack.c) and the script internalizer
// (lib/transaction_internalize.c) read at most four leading entries and stop
// at files.plist, so INSTALL and REMOVE must precede the two plists, exactly
// as xbps-create writes them. Placed after files.plist they would be
// extracted into the root filesystem as untracked payload.
func controlEntries(propsData, manifestData, installScript, removeScript []byte) []controlEntry {
	var entries []controlEntry
	if len(installScript) > 0 {
		entries = append(entries, controlEntry{name: "./INSTALL", data: installScript, mode: 0o755})
	}
	if len(removeScript) > 0 {
		entries = append(entries, controlEntry{name: "./REMOVE", data: removeScript, mode: 0o755})
	}
	entries = append(entries,
		controlEntry{name: "./props.plist", data: propsData, mode: 0o644},
		controlEntry{name: "./files.plist", data: manifestData, mode: 0o644},
	)
	return entries
}

// writeArchive writes the zstd-compressed tar stream: control entries first,
// then the payload in sorted order. Close errors are surfaced only when no
// earlier error occurred.
func writeArchive(w io.Writer, info *nfpm.Info, control []controlEntry) (err error) {
	zw, err := zstd.NewWriter(w)
	if err != nil {
		return fmt.Errorf("xbps: create zstd writer: %w", err)
	}
	tw := tar.NewWriter(zw)
	defer func() {
		if cerr := tw.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("xbps: close tar writer: %w", cerr)
		}
		if cerr := zw.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("xbps: close zstd writer: %w", cerr)
		}
	}()

	for _, entry := range control {
		if err := writeBytesEntry(tw, entry.name, entry.data, entry.mode, info); err != nil {
			return err
		}
	}
	for _, content := range sortedContents(info) {
		if content.Type == files.TypeRPMGhost {
			continue
		}
		if err := writeContentEntry(tw, content); err != nil {
			return err
		}
	}
	return nil
}

// Package writes a new xbps package to the given writer using the given info.
//
// When info.XBPS.Signature is configured, Package also writes a detached
// signature to info.Target + ".sig2". info.Target is set by the nfpm CLI; library
// callers that sign must set it themselves, otherwise an ErrSigningFailure is
// returned.
func (*XBPS) Package(info *nfpm.Info, w io.Writer) error {
	if info.Platform != "linux" {
		return fmt.Errorf("invalid platform: %s", info.Platform)
	}
	if err := removeStaleSidecar(info); err != nil {
		return err
	}
	var err error
	if info, err = ensureValidArch(info); err != nil {
		return err
	}
	if _, err := pkgver(info); err != nil {
		return err
	}
	if err := nfpm.PrepareForPackager(info, packagerName); err != nil {
		return err
	}

	props, err := propsManifest(info)
	if err != nil {
		return err
	}
	propsData, err := marshalPlist(props)
	if err != nil {
		return err
	}
	manifest, err := filesManifest(info)
	if err != nil {
		return err
	}
	manifestData, err := marshalPlist(manifest)
	if err != nil {
		return err
	}
	installScript, err := renderInstallScript(info)
	if err != nil {
		return err
	}
	removeScript, err := renderRemoveScript(info)
	if err != nil {
		return err
	}

	packageWriter := w
	var packageDigest func() []byte
	if packageShouldBeSigned(info) {
		if err := requireSignatureTarget(info); err != nil {
			return err
		}
		h := sha256.New()
		packageWriter = io.MultiWriter(w, h)
		packageDigest = func() []byte { return h.Sum(nil) }
	}

	if err := writeArchive(packageWriter, info, controlEntries(propsData, manifestData, installScript, removeScript)); err != nil {
		return err
	}
	if packageDigest != nil {
		if err := writeSignatureSidecar(info, packageDigest()); err != nil {
			return err
		}
	}
	return nil
}
