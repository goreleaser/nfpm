package xbps

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

var testMTime = time.Date(2023, 11, 5, 23, 15, 17, 0, time.UTC)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:            "foo",
		Arch:            "amd64",
		Version:         "v1.2.3",
		Prerelease:      "beta-1",
		VersionMetadata: "git-1",
		Release:         "2",
		Description:     "Foo does things\nAnd does them well.",
		Maintainer:      "Carlos A Becker <pkg@carlosbecker.com>",
		Homepage:        "https://example.com/foo",
		License:         "MIT",
		MTime:           testMTime,
		Overridables: nfpm.Overridables{
			Depends:   []string{"zlib>=1.0_1", "bash"},
			Provides:  []string{"foo-virtual-1.0_1"},
			Replaces:  []string{"foo-old>=1.0"},
			Conflicts: []string{"bar<2.0"},
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/usr/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        files.TypeConfig,
				},
				{
					Destination: "/var/lib/foo",
					Type:        files.TypeDir,
				},
				{
					Source:      "/etc/fake/fake.conf",
					Destination: "/etc/fake/fake-link.conf",
					Type:        files.TypeSymlink,
				},
			},
		},
	})
}

func readTarEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zstd.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer zr.Close()

	tr := tar.NewReader(zr)
	entries := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(tr)
		require.NoError(t, err)
		entries[hdr.Name] = body
	}
	return entries
}

func readTarNames(t *testing.T, data []byte) []string {
	t.Helper()
	zr, err := zstd.NewReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer zr.Close()

	tr := tar.NewReader(zr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names = append(names, hdr.Name)
		_, err = io.Copy(io.Discard, tr)
		require.NoError(t, err)
	}
	return names
}

func writeTempScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	target := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(target, []byte(body), 0o755))
	return target
}

func TestRegistered(t *testing.T) {
	packager, err := nfpm.Get(packagerName)
	require.NoError(t, err)
	require.Equal(t, Default, packager)
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".xbps", Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo()
	require.Equal(t, "foo-1.2.3.beta-1.git-1_2.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameDefaultRelease(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	require.Equal(t, "foo-1.2.3.beta-1.git-1_1.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameNoarch(t *testing.T) {
	info := exampleInfo()
	info.Arch = "all"
	require.Equal(t, "foo-1.2.3.beta-1.git-1_2.noarch.xbps", Default.ConventionalFileName(info))
}

func TestEnsureValidArchOverride(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Arch = "ppc64le"
	normalized, err := ensureValidArch(info)
	require.NoError(t, err)
	require.Equal(t, "ppc64le", normalized.Arch)
}

func TestEnsureValidArchMappings(t *testing.T) {
	testCases := map[string]string{
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

	for input, expected := range testCases {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = input
			normalized, err := ensureValidArch(info)
			require.NoError(t, err)
			require.Equal(t, expected, normalized.Arch)
		})
	}
}

func TestEnsureValidArchUnknown(t *testing.T) {
	info := exampleInfo()
	info.Arch = "loong64"
	_, err := ensureValidArch(info)
	require.ErrorContains(t, err, `unsupported architecture "loong64"`)
}

func TestVersionNormalizesLeadingVAndParts(t *testing.T) {
	info := exampleInfo()
	require.Equal(t, "1.2.3.beta-1.git-1", version(info))

	info.Prerelease = "-rc1-"
	info.VersionMetadata = ".build2."
	require.Equal(t, "1.2.3.rc1.build2", version(info))
}

func TestRevisionDefaultsToOneWhenEmpty(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	rev, err := revision(info)
	require.NoError(t, err)
	require.Equal(t, "1", rev)
}

func TestRevisionRejectsNonPositiveInteger(t *testing.T) {
	for _, release := range []string{"beta1", "0", "-1"} {
		release := release
		t.Run(release, func(t *testing.T) {
			info := exampleInfo()
			info.Release = release
			_, err := revision(info)
			require.ErrorContains(t, err, "must be a positive integer revision")
		})
	}
}

func TestPackageRejectsNonLinux(t *testing.T) {
	info := exampleInfo()
	info.Platform = "windows"

	err := Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, "invalid platform")
}

func TestPackageRejectsUnknownArch(t *testing.T) {
	info := exampleInfo()
	info.Arch = "loong64"

	err := Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, `unsupported architecture "loong64"`)
}

func TestPackageRejectsNonPositiveRelease(t *testing.T) {
	info := exampleInfo()
	info.Release = "0"

	err := Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, "must be a positive integer revision")
}

func TestPackageWritesZstdArchive(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	data := buf.Bytes()
	require.GreaterOrEqual(t, len(data), 4)
	require.Equal(t, []byte{0x28, 0xB5, 0x2F, 0xFD}, data[:4])

	entries := readTarEntries(t, data)
	require.Contains(t, entries, "./props.plist")
	require.Contains(t, entries, "./files.plist")
	require.Contains(t, entries, "./usr/bin/fake")
	require.Contains(t, entries, "./etc/fake/fake.conf")
	require.Contains(t, entries, "./etc/fake/fake-link.conf")
	require.Contains(t, entries, "./var/lib/foo/")
	require.Contains(t, string(entries["./props.plist"]), "foo-1.2.3.beta-1.git-1_2")
	require.Contains(t, string(entries["./props.plist"]), "Foo does things")
	require.Contains(t, string(entries["./files.plist"]), "/etc/fake/fake.conf")
}

func TestPackageControlEntriesComeFirst(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	names := readTarNames(t, buf.Bytes())
	require.GreaterOrEqual(t, len(names), 4)
	require.Equal(t, []string{"./props.plist", "./files.plist"}, names[:2])
	payload := append([]string(nil), names[2:]...)
	require.True(t, sort.StringsAreSorted(payload), "payload entries should be sorted after control entries: %v", payload)
}

func TestPackageWritesLifecycleScripts(t *testing.T) {
	info := exampleInfo()
	dir := t.TempDir()
	info.Scripts.PreInstall = writeTempScript(t, dir, "preinstall.sh", "echo preinstall\n")
	info.Scripts.PostInstall = writeTempScript(t, dir, "postinstall.sh", "echo postinstall\n")
	info.Scripts.PreRemove = writeTempScript(t, dir, "preremove.sh", "echo preremove\n")
	info.Scripts.PostRemove = writeTempScript(t, dir, "postremove.sh", "echo postremove\n")

	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	entries := readTarEntries(t, buf.Bytes())
	install := string(entries["./INSTALL"])
	remove := string(entries["./REMOVE"])

	require.Contains(t, install, "#!/bin/sh")
	require.Contains(t, install, "preinstall()")
	require.Contains(t, install, "postinstall()")
	require.Contains(t, install, "pre)")
	require.Contains(t, install, "post)")
	require.Contains(t, install, "echo preinstall")
	require.Contains(t, install, "echo postinstall")

	require.Contains(t, remove, "#!/bin/sh")
	require.Contains(t, remove, "preremove()")
	require.Contains(t, remove, "postremove()")
	require.Contains(t, remove, "pre)")
	require.Contains(t, remove, "post)")
	require.Contains(t, remove, "echo preremove")
	require.Contains(t, remove, "echo postremove")
}

func TestPackageOmitsLifecycleScriptsWhenUnset(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	entries := readTarEntries(t, buf.Bytes())
	require.NotContains(t, entries, "./INSTALL")
	require.NotContains(t, entries, "./REMOVE")
}

func TestPackageReturnsLifecycleScriptReadError(t *testing.T) {
	info := exampleInfo()
	info.Scripts.PreInstall = filepath.Join(t.TempDir(), "missing-preinstall.sh")

	err := Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, "missing-preinstall.sh")
}

func TestPropsManifestUsesGenericMetadata(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	props, err := propsManifest(info)
	require.NoError(t, err)
	require.Equal(t, "x86_64", props["architecture"])
	require.Equal(t, "foo", props["pkgname"])
	require.Equal(t, "foo-1.2.3.beta-1.git-1_2", props["pkgver"])
	require.Equal(t, "1.2.3.beta-1.git-1", props["version"])
	require.Equal(t, "Foo does things", props["short_desc"])
	require.Equal(t, "Foo does things\nAnd does them well.", props["long_desc"])
	require.Equal(t, "https://example.com/foo", props["homepage"])
	require.Equal(t, "MIT", props["license"])
	require.Equal(t, "Carlos A Becker <pkg@carlosbecker.com>", props["maintainer"])
	require.Equal(t, plistArray{"bash", "zlib>=1.0_1"}, props["run_depends"])
	require.Equal(t, plistArray{"bar<2.0"}, props["conflicts"])
	require.Equal(t, plistArray{"foo-virtual-1.0_1"}, props["provides"])
	require.Equal(t, plistArray{"foo-old>=1.0"}, props["replaces"])
	require.Equal(t, plistArray{"/etc/fake/fake.conf"}, props["conf_files"])
}

func TestPropsManifestIncludesAllConfigVariants(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/fake/a.conf",
			Type:        files.TypeConfigNoReplace,
		},
		&files.Content{
			Source:      "../testdata/whatever2.conf",
			Destination: "/etc/fake/b.conf",
			Type:        files.TypeConfigMissingOK,
		},
	)
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	props, err := propsManifest(info)
	require.NoError(t, err)
	require.Equal(t, plistArray{"/etc/fake/a.conf", "/etc/fake/b.conf", "/etc/fake/fake.conf"}, props["conf_files"])
}

func TestFilesManifestClassifiesPayloadEntries(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	manifest, err := filesManifest(info)
	require.NoError(t, err)
	require.Contains(t, manifest, "files")
	require.Contains(t, manifest, "conf_files")
	require.Contains(t, manifest, "links")
	require.Contains(t, manifest, "dirs")

	links := manifest["links"].(plistArray)
	require.Contains(t, links, plistDict{"file": "/etc/fake/fake-link.conf", "target": "/etc/fake/fake.conf"})
}

func TestMarshalPlistEscapesXMLDelimitersOnly(t *testing.T) {
	data, err := marshalPlist(plistDict{"long_desc": `<tag> & keep
quotes " ' untouched`})
	require.NoError(t, err)
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	require.Contains(t, text, "&lt;tag&gt; &amp; keep")
	require.Contains(t, text, `quotes " ' untouched`)
	require.NotContains(t, text, "&#xA;")
}
