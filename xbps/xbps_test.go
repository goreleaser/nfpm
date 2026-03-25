package xbps

import (
	"archive/tar"
	"bytes"
	"encoding/xml"
	"io"
		"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

var mtime = time.Date(2023, 11, 5, 23, 15, 17, 0, time.UTC)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things\nAnd does them well.",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "1.0.0",
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

func TestShortDescFallback(t *testing.T) {
	info := exampleInfo()
	info.XBPS.ShortDesc = ""
	require.Equal(t, "Foo does things", shortDesc(info))
}

func TestScripts(t *testing.T) {
	info := exampleInfo()
	installScript, err := renderInstallScript(info)
	require.NoError(t, err)
	require.Contains(t, string(installScript), "pre_install()")
	require.Contains(t, string(installScript), "post_install()")
	removeScript, err := renderRemoveScript(info)
	require.NoError(t, err)
	require.Contains(t, string(removeScript), "pre_remove()")
	require.Contains(t, string(removeScript), "post_remove()")
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
	require.Contains(t, props["alternatives"], "editor")
	confFiles := props["conf_files"].(plistArray)
	require.Len(t, confFiles, 1)
	require.Equal(t, "/etc/fake/fake.conf", confFiles[0])
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
	link := manifest["links"].(plistArray)[0].(plistDict)
	require.Equal(t, "/etc/fake/fake-link.conf", link["file"])
	require.Equal(t, "/etc/fake/fake.conf", link["target"])
}

func TestPackage(t *testing.T) {
	info := exampleInfo()
	var out bytes.Buffer
	err := Default.Package(info, &out)
	require.NoError(t, err)
	entries := readArchive(t, out.Bytes())
	require.Contains(t, entries, "./INSTALL")
	require.Contains(t, entries, "./REMOVE")
	require.Contains(t, entries, "./props.plist")
	require.Contains(t, entries, "./files.plist")
	require.Contains(t, entries, "./usr/bin/fake")
	require.Contains(t, entries, "./etc/fake/fake.conf")
	require.Contains(t, entries, "./etc/fake/fake-link.conf")
	require.NotContains(t, entries, "./var/lib/foo")

	props := parsePlistBytes(t, entries["./props.plist"])
	require.Equal(t, "foo-1.0.0.beta1_2", props["pkgver"])
	require.Equal(t, "Foo does things", props["short_desc"])
	filesPlist := parsePlistBytes(t, entries["./files.plist"])
	require.Contains(t, filesPlist, "conf_files")
	require.Contains(t, filesPlist, "links")
}

func TestPackageRejectsNonLinux(t *testing.T) {
	info := exampleInfo()
	info.Platform = "darwin"
	err := Default.Package(info, io.Discard)
	require.Error(t, err)
}

func readArchive(t *testing.T, data []byte) map[string][]byte {
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

type plistXML struct {
	XMLName xml.Name `xml:"plist"`
	Inner   plistAny `xml:",any"`
}

type plistAny struct {
	XMLName xml.Name
	Nodes   []plistNode `xml:",any"`
	Text    string      `xml:",chardata"`
}

type plistNode struct {
	XMLName xml.Name
	Nodes   []plistNode `xml:",any"`
	Text    string      `xml:",chardata"`
}

func parsePlistBytes(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc plistXML
	require.NoError(t, xml.Unmarshal(data, &doc))
	root := plistNode{XMLName: doc.Inner.XMLName, Nodes: doc.Inner.Nodes, Text: doc.Inner.Text}
	parsed, ok := plistNodeValue(root).(map[string]any)
	require.True(t, ok)
	return parsed
}

func plistNodeValue(node plistNode) any {
	switch node.XMLName.Local {
	case "dict":
		result := map[string]any{}
		for i := 0; i < len(node.Nodes); i += 2 {
			key := strings.TrimSpace(node.Nodes[i].Text)
			result[key] = plistNodeValue(node.Nodes[i+1])
		}
		return result
	case "array":
		result := make([]any, 0, len(node.Nodes))
		for _, child := range node.Nodes {
			result = append(result, plistNodeValue(child))
		}
		return result
	case "string":
		return strings.TrimSpace(node.Text)
	case "integer":
		return strings.TrimSpace(node.Text)
	case "true":
		return true
	case "false":
		return false
	default:
		return strings.TrimSpace(node.Text)
	}
}

func TestFileEntryRegularHasHashAndSize(t *testing.T) {
	info := exampleInfo()
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))
	var regular *files.Content
	for _, content := range info.Contents {
		if content.Destination == "/usr/bin/fake" {
			regular = content
			break
		}
	}
	require.NotNil(t, regular)
	entry, err := fileEntry(regular, false)
	require.NoError(t, err)
	require.Equal(t, "/usr/bin/fake", entry["file"])
	require.NotEmpty(t, entry["sha256"])
	require.NotEmpty(t, entry["size"])
}
