// Package xbps implements nfpm.Packager providing .xbps bindings.
package xbps

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/klauspost/compress/zstd"
)

const packagerName = "xbps"

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

func revision(info *nfpm.Info) (string, error) {
	trimmed := strings.TrimSpace(info.Release)
	if trimmed == "" {
		return "1", nil
	}
	if _, err := strconv.Atoi(trimmed); err != nil {
		return "", fmt.Errorf("xbps: release %q must be a positive integer (XBPS revision)", info.Release)
	}
	return trimmed, nil
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

func pkgver(info *nfpm.Info) (string, error) {
	rev, err := revision(info)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s_%s", info.Name, version(info), rev), nil
}

func shortDesc(info *nfpm.Info) string {
	if info.XBPS.ShortDesc != "" {
		return strings.TrimSpace(info.XBPS.ShortDesc)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(info.Description), "\n")
	return strings.TrimSpace(first)
}

func normalizeTargetForMetadata(dst, src string) string {
	if strings.HasPrefix(src, "/") {
		return files.NormalizeAbsoluteFilePath(src)
	}
	return files.NormalizeAbsoluteFilePath(path.Join(path.Dir(dst), src))
}

func sortedContents(info *nfpm.Info) files.Contents {
	contents := slices.Clone(info.Contents)
	sort.Sort(contents)
	return contents
}

func isConfigType(contentType string) bool {
	switch contentType {
	case files.TypeConfig, files.TypeConfigNoReplace, files.TypeConfigMissingOK:
		return true
	default:
		return false
	}
}

func isRegularType(contentType string) bool {
	switch contentType {
	case files.TypeDir, files.TypeImplicitDir, files.TypeSymlink, files.TypeRPMGhost:
		return false
	default:
		return true
	}
}

func configFiles(info *nfpm.Info) []string {
	var result []string
	for _, content := range sortedContents(info) {
		if isConfigType(content.Type) {
			result = append(result, files.NormalizeAbsoluteFilePath(content.Destination))
		}
	}
	return result
}

func installedSize(info *nfpm.Info) uint64 {
	var total uint64
	for _, content := range sortedContents(info) {
		if isRegularType(content.Type) {
			total += uint64(content.Size())
		}
	}
	return total
}

func sortedStrings(values []string) []string {
	result := slices.Clone(values)
	sort.Strings(result)
	return result
}

func alternatives(info *nfpm.Info) (map[string][]string, error) {
	if len(info.XBPS.Alternatives) == 0 {
		return nil, nil
	}
	result := map[string][]string{}
	for _, alt := range info.XBPS.Alternatives {
		if strings.TrimSpace(alt.Group) == "" || strings.TrimSpace(alt.LinkName) == "" || strings.TrimSpace(alt.Target) == "" {
			return nil, fmt.Errorf("xbps: alternatives require group, link_name, and target")
		}
		result[alt.Group] = append(result[alt.Group], alt.LinkName+":"+alt.Target)
	}
	for key := range result {
		sort.Strings(result[key])
	}
	return result, nil
}

func portablePropEscapeText(buf *bytes.Buffer, value string) {
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

func renderEmbeddedScript(source string) ([]byte, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func renderInstallScript(info *nfpm.Info) ([]byte, error) {
	preScript, err := loadOptionalScript(info.Scripts.PreInstall)
	if err != nil {
		return nil, err
	}
	postScript, err := loadOptionalScript(info.Scripts.PostInstall)
	if err != nil {
		return nil, err
	}
	if len(preScript) == 0 && len(postScript) == 0 {
		return nil, nil
	}
	return wrapXBPSInstallScript(preScript, postScript), nil
}

func renderRemoveScript(info *nfpm.Info) ([]byte, error) {
	preScript, err := loadOptionalScript(info.Scripts.PreRemove)
	if err != nil {
		return nil, err
	}
	postScript, err := loadOptionalScript(info.Scripts.PostRemove)
	if err != nil {
		return nil, err
	}
	if len(preScript) == 0 && len(postScript) == 0 {
		return nil, nil
	}
	return wrapXBPSRemoveScript(preScript, postScript), nil
}

func loadOptionalScript(source string) ([]byte, error) {
	if source == "" {
		return nil, nil
	}
	return renderEmbeddedScript(source)
}

func appendHereDocScript(buf *bytes.Buffer, label string, data []byte) {
	fmt.Fprintf(buf, "cat >\"$script_dir/%s\" <<'__NFPM_%s__'\n", label, label)
	buf.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		buf.WriteByte('\n')
	}
	fmt.Fprintf(buf, "__NFPM_%s__\nchmod 0755 \"$script_dir/%s\"\n\n", label, label)
}

func writeWrapperPreamble(buf *bytes.Buffer) {
	buf.WriteString("#!/bin/sh\n\n")
	buf.WriteString("script_dir=$(mktemp -d)\n")
	buf.WriteString("cleanup() {\n    rm -rf \"$script_dir\"\n}\n")
	buf.WriteString("trap cleanup EXIT INT TERM\n\n")
	buf.WriteString(`run_script() {
    script_name="$1"
    semantic_action="$2"
    shift 2
    /bin/sh "$script_dir/$script_name" "$semantic_action" "$@"
}

`)
}

func wrapXBPSInstallScript(preInstall, postInstall []byte) []byte {
	var buf bytes.Buffer
	writeWrapperPreamble(&buf)
	if len(preInstall) > 0 {
		appendHereDocScript(&buf, "pre_install", preInstall)
	}
	if len(postInstall) > 0 {
		appendHereDocScript(&buf, "post_install", postInstall)
	}
	buf.WriteString(`case "$1:$4" in
    pre:no)
        if [ -x "$script_dir/pre_install" ]; then
            run_script pre_install install
        fi
        ;;
    pre:yes)
        if [ -x "$script_dir/pre_install" ]; then
            run_script pre_install upgrade
        fi
        ;;
    post:no)
        if [ -x "$script_dir/post_install" ]; then
            run_script post_install install
        fi
        ;;
    post:yes)
        if [ -x "$script_dir/post_install" ]; then
            run_script post_install upgrade
        fi
        ;;
esac

exit 0
`)
	return buf.Bytes()
}

func wrapXBPSRemoveScript(preRemove, postRemove []byte) []byte {
	var buf bytes.Buffer
	writeWrapperPreamble(&buf)
	if len(preRemove) > 0 {
		appendHereDocScript(&buf, "pre_remove", preRemove)
	}
	if len(postRemove) > 0 {
		appendHereDocScript(&buf, "post_remove", postRemove)
	}
	buf.WriteString(`case "$1:$4" in
    pre:no)
        if [ -x "$script_dir/pre_remove" ]; then
            run_script pre_remove remove
        fi
        ;;
    pre:yes)
        if [ -x "$script_dir/pre_remove" ]; then
            run_script pre_remove upgrade
        fi
        ;;
    post:no)
        if [ -x "$script_dir/post_remove" ]; then
            run_script post_remove remove
        fi
        ;;
    purge:no)
        if [ -x "$script_dir/post_remove" ]; then
            run_script post_remove purge
        fi
        ;;
esac

exit 0
`)
	return buf.Bytes()
}

type plistValue any

type plistDict map[string]plistValue

type plistArray []plistValue

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
			portablePropEscapeText(buf, key)
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
		portablePropEscapeText(buf, v)
		buf.WriteString("</string>")
	case bool:
		if v {
			buf.WriteString("<true/>")
		} else {
			buf.WriteString("<false/>")
		}
	case uint64:
		fmt.Fprintf(buf, "<integer>%d</integer>", v)
	case int:
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

func fileEntry(content *files.Content) (plistDict, error) {
	entry := plistDict{
		"file": files.NormalizeAbsoluteFilePath(content.Destination),
	}
	if content.Type == files.TypeSymlink {
		entry["target"] = normalizeTargetForMetadata(content.Destination, content.Source)
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
		switch {
		case content.Type == files.TypeRPMGhost:
			continue
		case content.Type == files.TypeDir || content.Type == files.TypeImplicitDir:
			entry, err := fileEntry(content)
			if err != nil {
				return nil, err
			}
			dirs = append(dirs, entry)
		case content.Type == files.TypeSymlink:
			entry, err := fileEntry(content)
			if err != nil {
				return nil, err
			}
			links = append(links, entry)
		case isConfigType(content.Type):
			entry, err := fileEntry(content)
			if err != nil {
				return nil, err
			}
			configs = append(configs, entry)
		default:
			entry, err := fileEntry(content)
			if err != nil {
				return nil, err
			}
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

// normalizeDep ensures a dependency string is in valid XBPS format.
// Bare package names (e.g. "bash") are converted to "bash>=0".
func normalizeDep(dep string) string {
	for _, op := range []string{">=", "<=", ">", "<"} {
		if strings.Contains(dep, op) {
			return dep
		}
	}
	return dep + ">=0"
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
	if info.Homepage != "" {
		manifest["homepage"] = info.Homepage
	}
	if info.License != "" {
		manifest["license"] = info.License
	}
	if info.Maintainer != "" {
		manifest["maintainer"] = info.Maintainer
	}
	if info.Description != "" {
		manifest["long_desc"] = info.Description
	}
	if info.XBPS.Preserve {
		manifest["preserve"] = true
	}
	if len(info.Depends) > 0 {
		deps := plistArray{}
		for _, value := range sortedStrings(info.Depends) {
			deps = append(deps, normalizeDep(value))
		}
		manifest["run_depends"] = deps
	}
	if confs := configFiles(info); len(confs) > 0 {
		items := plistArray{}
		for _, value := range confs {
			items = append(items, value)
		}
		manifest["conf_files"] = items
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
		items := plistArray{}
		for _, value := range sortedStrings(values) {
			items = append(items, value)
		}
		manifest[key] = items
	}
	if len(info.XBPS.Tags) > 0 {
		manifest["tags"] = strings.Join(sortedStrings(info.XBPS.Tags), " ")
	}
	alts, err := alternatives(info)
	if err != nil {
		return nil, err
	}
	if len(alts) > 0 {
		altDict := plistDict{}
		keys := make([]string, 0, len(alts))
		for key := range alts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			items := plistArray{}
			for _, value := range alts[key] {
				items = append(items, value)
			}
			altDict[key] = items
		}
		manifest["alternatives"] = altDict
	}
	return manifest, nil
}

func (*XBPS) ConventionalFileName(info *nfpm.Info) string {
	copyInfo := *info
	normalized, err := ensureValidArch(&copyInfo)
	if err != nil {
		rev, _ := revision(info)
		if rev == "" {
			rev = "1"
		}
		return fmt.Sprintf("%s-%s_%s.%s.xbps", info.Name, version(info), rev, info.Arch)
	}
	pv, err := pkgver(normalized)
	if err != nil {
		return fmt.Sprintf("%s-%s_1.%s.xbps", info.Name, version(info), normalized.Arch)
	}
	return fmt.Sprintf("%s.%s.xbps", pv, normalized.Arch)
}

// ConventionalExtension returns the file name conventionally used for XBPS packages.
func (*XBPS) ConventionalExtension() string {
	return ".xbps"
}

func writeBytesEntry(tw *tar.Writer, name string, data []byte, mode int64, info nfpm.Info) error {
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

func writeContentEntry(tw *tar.Writer, content *files.Content) error {
	name := files.AsExplicitRelativePath(content.Destination)
	if content.Type == files.TypeSymlink {
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
	}
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

// Package writes a new xbps package to the given writer using the given info.
func (*XBPS) Package(info *nfpm.Info, w io.Writer) error {
	if info.Platform != "linux" {
		return fmt.Errorf("invalid platform: %s", info.Platform)
	}
	var err error
	if info, err = ensureValidArch(info); err != nil {
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
	zw, err := zstd.NewWriter(w)
	if err != nil {
		return fmt.Errorf("xbps: create zstd writer: %w", err)
	}
	defer zw.Close()
	tw := tar.NewWriter(zw)
	defer tw.Close()
	if len(installScript) > 0 {
		if err := writeBytesEntry(tw, "./INSTALL", installScript, 0o755, *info); err != nil {
			return err
		}
	}
	if len(removeScript) > 0 {
		if err := writeBytesEntry(tw, "./REMOVE", removeScript, 0o755, *info); err != nil {
			return err
		}
	}
	if err := writeBytesEntry(tw, "./props.plist", propsData, 0o644, *info); err != nil {
		return err
	}
	if err := writeBytesEntry(tw, "./files.plist", manifestData, 0o644, *info); err != nil {
		return err
	}
	for _, content := range sortedContents(info) {
		switch content.Type {
		case files.TypeDir, files.TypeImplicitDir, files.TypeRPMGhost:
			continue
		default:
			if err := writeContentEntry(tw, content); err != nil {
				return err
			}
		}
	}
	return nil
}
