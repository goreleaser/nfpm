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
	for _, content := range sortedContents(info) {
		if isPayloadFileType(content.Type) {
			total += uint64(content.Size())
		}
	}
	return total
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

func stringsToPlistArray(values []string) plistArray {
	items := plistArray{}
	for _, value := range sortedStrings(values) {
		items = append(items, value)
	}
	return items
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
		manifest["run_depends"] = stringsToPlistArray(info.Depends)
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

type xbpsScriptlet struct {
	action string
	name   string
	source string
}

func loadOptionalScript(source string) ([]byte, error) {
	if source == "" {
		return nil, nil
	}
	return os.ReadFile(source)
}

func appendScriptFunction(buf *bytes.Buffer, name string, data []byte) {
	fmt.Fprintf(buf, "%s() {\n", name)
	buf.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n\n")
}

func renderXBPSActionScript(scriptlets []xbpsScriptlet) ([]byte, error) {
	var loaded []xbpsScriptlet
	bodies := map[string][]byte{}
	for _, scriptlet := range scriptlets {
		data, err := loadOptionalScript(scriptlet.source)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			continue
		}
		loaded = append(loaded, scriptlet)
		bodies[scriptlet.name] = data
	}
	if len(loaded) == 0 {
		return nil, nil
	}

	var buf bytes.Buffer
	buf.WriteString("#!/bin/sh\n\n")
	for _, scriptlet := range loaded {
		appendScriptFunction(&buf, scriptlet.name, bodies[scriptlet.name])
	}

	buf.WriteString("case \"$1\" in\n")
	for _, scriptlet := range loaded {
		fmt.Fprintf(&buf, "%s)\n\t%s\n\t;;\n", scriptlet.action, scriptlet.name)
	}
	buf.WriteString("esac\n")
	return buf.Bytes(), nil
}

func renderInstallScript(info *nfpm.Info) ([]byte, error) {
	return renderXBPSActionScript([]xbpsScriptlet{
		{action: "pre", name: "preinstall", source: info.Scripts.PreInstall},
		{action: "post", name: "postinstall", source: info.Scripts.PostInstall},
	})
}

func renderRemoveScript(info *nfpm.Info) ([]byte, error) {
	return renderXBPSActionScript([]xbpsScriptlet{
		{action: "pre", name: "preremove", source: info.Scripts.PreRemove},
		{action: "post", name: "postremove", source: info.Scripts.PostRemove},
	})
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

func writeSignatureSidecar(info *nfpm.Info, digest []byte) error {
	signature, err := signPackageDigest(info, digest)
	if err != nil {
		return &nfpm.ErrSigningFailure{Err: err}
	}
	if err := os.WriteFile(info.Target+".sig2", signature, 0o644); err != nil {
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

// Package writes a new xbps package to the given writer using the given info.
func (*XBPS) Package(info *nfpm.Info, w io.Writer) error {
	if info.Platform != "linux" {
		return fmt.Errorf("invalid platform: %s", info.Platform)
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

	zw, err := zstd.NewWriter(packageWriter)
	if err != nil {
		return fmt.Errorf("xbps: create zstd writer: %w", err)
	}
	tw := tar.NewWriter(zw)
	if err := writeBytesEntry(tw, "./props.plist", propsData, 0o644, info); err != nil {
		_ = tw.Close()
		_ = zw.Close()
		return err
	}
	if err := writeBytesEntry(tw, "./files.plist", manifestData, 0o644, info); err != nil {
		_ = tw.Close()
		_ = zw.Close()
		return err
	}
	if len(installScript) > 0 {
		if err := writeBytesEntry(tw, "./INSTALL", installScript, 0o755, info); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return err
		}
	}
	if len(removeScript) > 0 {
		if err := writeBytesEntry(tw, "./REMOVE", removeScript, 0o755, info); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return err
		}
	}
	for _, content := range sortedContents(info) {
		if content.Type == files.TypeRPMGhost {
			continue
		}
		if err := writeContentEntry(tw, content); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = zw.Close()
		return fmt.Errorf("xbps: close tar writer: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("xbps: close zstd writer: %w", err)
	}
	if packageDigest != nil {
		if err := writeSignatureSidecar(info, packageDigest()); err != nil {
			return err
		}
	}
	return nil
}
