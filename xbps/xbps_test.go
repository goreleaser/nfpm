package xbps

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
)

var mtime = time.Date(2023, 11, 5, 23, 15, 17, 0, time.UTC)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things\nAnd does them well.",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "v1.0.0",
		Prerelease:  "beta1",
		Release:     "2",
		Homepage:    "http://carlosbecker.com",
		License:     "MIT",
		MTime:       mtime,
		Overridables: nfpm.Overridables{
			Depends:   []string{"bash"},
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
			Scripts: nfpm.Scripts{
				PreInstall:  "../testdata/scripts/preinstall.sh",
				PostInstall: "../testdata/scripts/postinstall.sh",
				PreRemove:   "../testdata/scripts/preremove.sh",
				PostRemove:  "../testdata/scripts/postremove.sh",
			},
			XBPS: nfpm.XBPS{
				Preserve:  true,
				Tags:      []string{"cli", "network"},
				Reverts:   []string{"1.0_1"},
				ShortDesc: "Foo does things",
				Alternatives: []nfpm.XBPSAlternative{
					{Group: "editor", LinkName: "/usr/bin/editor", Target: "/usr/bin/fake"},
				},
			},
		},
	})
}

func readTarEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
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

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".xbps", Default.ConventionalExtension())
}

func readTarNames(t *testing.T, data []byte) []string {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(data))
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

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo()
	require.Equal(t, "foo-1.0.0.beta1_2.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameDefaultRelease(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	require.Equal(t, "foo-1.0.0.beta1_1.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameNoArch(t *testing.T) {
	info := exampleInfo()
	info.Arch = "all"
	require.Equal(t, "foo-1.0.0.beta1_2.noarch.xbps", Default.ConventionalFileName(info))
}

func TestEnsureValidArchOverride(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Arch = "ppc-musl"
	normalized, err := ensureValidArch(info)
	require.NoError(t, err)
	require.Equal(t, "ppc-musl", normalized.Arch)
}

func TestEnsureValidArchUnknown(t *testing.T) {
	info := exampleInfo()
	info.Arch = "loong64"
	_, err := ensureValidArch(info)
	require.Error(t, err)
}

func TestVersionNormalizesLeadingV(t *testing.T) {
	info := exampleInfo()
	info.Prerelease = ""
	require.Equal(t, "1.0.0", version(info))
	require.Equal(t, "foo-1.0.0_2", pkgver(info))
}

func TestShortDescFallback(t *testing.T) {
	info := exampleInfo()
	info.XBPS.ShortDesc = ""
	require.Equal(t, "Foo does things", shortDesc(info))
}

func TestScripts(t *testing.T) {
	info := exampleInfo()
	installScript, err := renderInstallScript(info)
	require.NoError(t, err)
	require.Contains(t, string(installScript), `case "$1:$4" in`)
	require.Contains(t, string(installScript), `run_script post_install install`)
	require.Contains(t, string(installScript), `run_script post_install upgrade`)
	require.NotContains(t, string(installScript), `set -e`)

	removeScript, err := renderRemoveScript(info)
	require.NoError(t, err)
	require.Contains(t, string(removeScript), `run_script pre_remove remove`)
	require.Contains(t, string(removeScript), `run_script post_remove purge`)
}

func TestAlternativesValidation(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Alternatives = append(info.XBPS.Alternatives, nfpm.XBPSAlternative{Group: "broken"})
	_, err := alternatives(info)
	require.Error(t, err)
}

func TestPropsManifestSortsMetadataAndGroupsAlternatives(t *testing.T) {
	info := exampleInfo()
	info.Depends = []string{"zlib>=1.0_1", "bash>=5.0_1"}
	info.Provides = []string{"foo-virtual-2.0_1", "foo-virtual-1.0_1"}
	info.Conflicts = []string{"zzz<1.0", "aaa<1.0"}
	info.Replaces = []string{"foo-old>=2.0", "foo-old>=1.0"}
	info.XBPS.Reverts = []string{"2.0_1", "1.0_1"}
	info.XBPS.Tags = []string{"network", "cli"}
	info.XBPS.Alternatives = []nfpm.XBPSAlternative{
		{Group: "pager", LinkName: "/usr/bin/pager", Target: "/usr/bin/fake"},
		{Group: "editor", LinkName: "/usr/bin/vi", Target: "/usr/bin/fake"},
		{Group: "editor", LinkName: "/usr/bin/editor", Target: "/usr/bin/fake"},
	}

	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))
	props, err := propsManifest(info)
	require.NoError(t, err)

	require.Equal(t, plistArray{"bash>=5.0_1", "zlib>=1.0_1"}, props["run_depends"])
	require.Equal(t, plistArray{"aaa<1.0", "zzz<1.0"}, props["conflicts"])
	require.Equal(t, plistArray{"foo-virtual-1.0_1", "foo-virtual-2.0_1"}, props["provides"])
	require.Equal(t, plistArray{"foo-old>=1.0", "foo-old>=2.0"}, props["replaces"])
	require.Equal(t, plistArray{"1.0_1", "2.0_1"}, props["reverts"])
	require.Equal(t, "cli network", props["tags"])

	alts := props["alternatives"].(plistDict)
	require.Equal(t, plistArray{"/usr/bin/editor:/usr/bin/fake", "/usr/bin/vi:/usr/bin/fake"}, alts["editor"])
	require.Equal(t, plistArray{"/usr/bin/pager:/usr/bin/fake"}, alts["pager"])
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

func TestFilesManifestIgnoresGhostEntries(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/var/lib/foo.ghost",
		Type:        files.TypeRPMGhost,
	})

	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))
	manifest, err := filesManifest(info)
	require.NoError(t, err)
	data, err := marshalPlist(manifest)
	require.NoError(t, err)
	require.NotContains(t, string(data), "/var/lib/foo.ghost")
}

func TestPackageRejectsNonLinux(t *testing.T) {
	info := exampleInfo()
	info.Platform = "windows"
	var buf bytes.Buffer
	err := Default.Package(info, &buf)
	require.ErrorContains(t, err, "invalid platform")
}

func TestPackageWithoutScriptsOmitsLifecycleWrappers(t *testing.T) {
	info := exampleInfo()
	info.Scripts = nfpm.Scripts{}
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	entries := readTarEntries(t, buf.Bytes())
	require.NotContains(t, entries, "./INSTALL")
	require.NotContains(t, entries, "./REMOVE")
	require.Contains(t, entries, "./props.plist")
	require.Contains(t, entries, "./files.plist")
}

func TestPackageControlEntriesComeFirst(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	names := readTarNames(t, buf.Bytes())
	require.GreaterOrEqual(t, len(names), 4)
	require.Equal(t, []string{"./INSTALL", "./REMOVE", "./props.plist", "./files.plist"}, names[:4])
}

func TestPropsManifest(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))
	props, err := propsManifest(info)
	require.NoError(t, err)
	require.Equal(t, "x86_64", props["architecture"])
	require.Equal(t, "foo-1.0.0.beta1_2", props["pkgver"])
	require.Equal(t, "Foo does things", props["short_desc"])
	require.Equal(t, true, props["preserve"])
	require.Equal(t, "cli network", props["tags"])
	confFiles := props["conf_files"].(plistArray)
	require.Len(t, confFiles, 1)
	require.Equal(t, "/etc/fake/fake.conf", confFiles[0])
}

func TestMarshalPlistKeepsLiteralNewlines(t *testing.T) {
	data, err := marshalPlist(plistDict{"long_desc": "line1\nline2\n"})
	require.NoError(t, err)
	text := string(data)
	require.NotContains(t, text, "&#xA;")
	require.Contains(t, text, "line1\nline2\n")
}

func TestFilesManifest(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))
	manifest, err := filesManifest(info)
	require.NoError(t, err)
	require.Contains(t, manifest, "files")
	require.Contains(t, manifest, "conf_files")
	require.Contains(t, manifest, "links")
	require.Contains(t, manifest, "dirs")
}

func TestPackage(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	entries := readTarEntries(t, buf.Bytes())

	require.Contains(t, entries, "./INSTALL")
	require.Contains(t, entries, "./REMOVE")
	require.Contains(t, entries, "./props.plist")
	require.Contains(t, entries, "./files.plist")
	require.Contains(t, entries, "./usr/bin/fake")
	require.Contains(t, entries, "./etc/fake/fake.conf")
	require.Contains(t, entries, "./etc/fake/fake-link.conf")

	require.Contains(t, string(entries["./props.plist"]), "foo-1.0.0.beta1_2")
	require.Contains(t, string(entries["./INSTALL"]), `run_script post_install install`)
	require.NotContains(t, string(entries["./props.plist"]), "&#xA;")
}

func TestPortablePropEscapeText(t *testing.T) {
	var buf bytes.Buffer
	portablePropEscapeText(&buf, `<tag> & keep
quotes " ' untouched`)
	require.Equal(t, "&lt;tag&gt; &amp; keep\nquotes \" ' untouched", strings.ReplaceAll(buf.String(), "\r\n", "\n"))
}
