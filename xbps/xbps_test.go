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

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo()
	require.Equal(t, "foo-1.0.0.beta1_2.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameDefaultRelease(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	require.Equal(t, "foo-1.0.0.beta1_1.x86_64.xbps", Default.ConventionalFileName(info))
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
