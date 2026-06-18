package xbps

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
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

func packageToTargetWithError(t *testing.T, info *nfpm.Info) (string, error) {
	t.Helper()
	target := filepath.Join(t.TempDir(), Default.ConventionalFileName(info))
	info.Target = target
	f, err := os.Create(target)
	require.NoError(t, err)
	packageErr := Default.Package(info, f)
	closeErr := f.Close()
	if packageErr != nil {
		return target, packageErr
	}
	return target, closeErr
}

func packageToTarget(t *testing.T, info *nfpm.Info) string {
	t.Helper()
	target, err := packageToTargetWithError(t, info)
	require.NoError(t, err)
	return target
}

func requireSignatureSidecarVerifies(t *testing.T, target, publicKey string) {
	t.Helper()
	packageData, err := os.ReadFile(target)
	require.NoError(t, err)
	digest := sha256.Sum256(packageData)
	signature, err := os.ReadFile(target + ".sig2")
	require.NoError(t, err)
	require.NotEmpty(t, signature)
	require.NoError(t, sign.RSAVerifySHA256Digest(digest[:], signature, publicKey))
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

func TestPackageDoesNotWriteSignatureSidecarWhenUnsigned(t *testing.T) {
	info := exampleInfo()
	target := packageToTarget(t, info)

	_, err := os.Stat(target + ".sig2")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPackageWritesSignatureSidecar(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Signature.KeyFile = "../internal/sign/testdata/rsa_unprotected.priv"

	target := packageToTarget(t, info)
	requireSignatureSidecarVerifies(t, target, "../internal/sign/testdata/rsa_unprotected.pub")
}

func TestPackageWritesEncryptedSignatureSidecar(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Signature.KeyFile = "../internal/sign/testdata/rsa.priv"
	info.XBPS.Signature.KeyPassphrase = "hunter2"

	target := packageToTarget(t, info)
	requireSignatureSidecarVerifies(t, target, "../internal/sign/testdata/rsa.pub")
}

func TestPackageWritesSignatureSidecarWithInjectedSigner(t *testing.T) {
	info := exampleInfo()
	var signedDigest []byte
	info.XBPS.Signature.SignFn = func(data io.Reader) ([]byte, error) {
		var err error
		signedDigest, err = io.ReadAll(data)
		return []byte("injected-signature"), err
	}

	target := packageToTarget(t, info)
	packageData, err := os.ReadFile(target)
	require.NoError(t, err)
	digest := sha256.Sum256(packageData)
	require.Equal(t, digest[:], signedDigest)

	sidecar, err := os.ReadFile(target + ".sig2")
	require.NoError(t, err)
	require.Equal(t, "injected-signature", string(sidecar))
}

func TestPackageRequiresTargetPathForSignatureSidecar(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Signature.KeyFile = "../internal/sign/testdata/rsa_unprotected.priv"

	err := Default.Package(info, &bytes.Buffer{})
	var signingErr *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &signingErr)
	require.ErrorContains(t, err, "target path required for signature sidecar")
}

func TestPackageReturnsSigningFailure(t *testing.T) {
	invalidKey := filepath.Join(t.TempDir(), "invalid.pem")
	require.NoError(t, os.WriteFile(invalidKey, []byte("not a pem"), 0o600))

	testCases := map[string]struct {
		keyFile string
		wantErr string
	}{
		"missing key": {filepath.Join(t.TempDir(), "missing.pem"), "reading key file"},
		"invalid key": {invalidKey, "no PEM block found"},
	}

	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			info := exampleInfo()
			info.XBPS.Signature.KeyFile = testCase.keyFile

			_, err := packageToTargetWithError(t, info)
			var signingErr *nfpm.ErrSigningFailure
			require.ErrorAs(t, err, &signingErr)
			require.ErrorContains(t, err, testCase.wantErr)
		})
	}
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

func TestPropsManifestUsesXBPSMetadata(t *testing.T) {
	info := exampleInfo()
	info.XBPS.ShortDesc = "Explicit XBPS summary"
	info.XBPS.Preserve = true
	info.XBPS.Tags = []string{"utilities", "cli"}
	info.XBPS.Reverts = []string{"1.2.2_1", "1.2.1_1"}
	info.XBPS.Alternatives = []nfpm.XBPSAlternative{
		{Group: "editor", LinkName: "/usr/bin/view", Target: "/usr/bin/foo-view"},
		{Group: "editor", LinkName: "/usr/bin/edit", Target: "/usr/bin/foo"},
		{Group: "pager", LinkName: "/usr/bin/page", Target: "/usr/bin/foo-page"},
	}
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	props, err := propsManifest(info)
	require.NoError(t, err)
	require.Equal(t, "Explicit XBPS summary", props["short_desc"])
	require.Equal(t, true, props["preserve"])
	require.Equal(t, "cli utilities", props["tags"])
	require.Equal(t, plistArray{"1.2.1_1", "1.2.2_1"}, props["reverts"])
	require.Equal(t, plistDict{
		"editor": plistArray{"/usr/bin/edit:/usr/bin/foo", "/usr/bin/view:/usr/bin/foo-view"},
		"pager":  plistArray{"/usr/bin/page:/usr/bin/foo-page"},
	}, props["alternatives"])
}

func TestPropsManifestRejectsMalformedAlternatives(t *testing.T) {
	testCases := map[string]nfpm.XBPSAlternative{
		"empty group":      {LinkName: "/usr/bin/foo", Target: "/usr/bin/foo-tool"},
		"group delimiter":  {Group: "foo:bar", LinkName: "/usr/bin/foo", Target: "/usr/bin/foo-tool"},
		"group whitespace": {Group: "foo bar", LinkName: "/usr/bin/foo", Target: "/usr/bin/foo-tool"},
		"empty link":       {Group: "foo", Target: "/usr/bin/foo-tool"},
		"link delimiter":   {Group: "foo", LinkName: "/usr/bin/foo:alt", Target: "/usr/bin/foo-tool"},
		"empty target":     {Group: "foo", LinkName: "/usr/bin/foo"},
		"target delimiter": {Group: "foo", LinkName: "/usr/bin/foo", Target: "/usr/bin/foo:tool"},
	}

	for name, alt := range testCases {
		name, alt := name, alt
		t.Run(name, func(t *testing.T) {
			info := exampleInfo()
			info.XBPS.Alternatives = []nfpm.XBPSAlternative{alt}
			require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

			_, err := propsManifest(info)
			require.ErrorContains(t, err, "xbps: invalid alternative")
		})
	}
}

func TestPropsManifestIncludesAllConfigVariants(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(
		info.Contents,
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

func TestSymlinkMetadataMatchesTarPayload(t *testing.T) {
	// Regression test: for a relative symlink target the files.plist
	// "target" must equal the value written into the tar header
	// (Linkname: content.Source). Previously the metadata was normalized
	// to an absolute path while the payload kept the raw relative target,
	// which made xbps-pkgdb report the link as modified.
	for _, src := range []string{"../lib/libfoo.so.1", "/usr/lib/libfoo.so.1", "libfoo.so.1"} {
		content := &files.Content{
			Source:      src,
			Destination: "/usr/lib/foo/libfoo.so",
			Type:        files.TypeSymlink,
			FileInfo:    &files.ContentFileInfo{MTime: testMTime},
		}

		entry, err := fileEntry(content)
		require.NoError(t, err)
		require.Equal(t, src, entry["target"], "files.plist target must equal raw Source")

		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		require.NoError(t, writeContentEntry(tw, content))
		require.NoError(t, tw.Close())

		tr := tar.NewReader(&buf)
		hdr, err := tr.Next()
		require.NoError(t, err)
		require.Equal(t, hdr.Linkname, entry["target"], "tar Linkname and files.plist target must agree")
	}
}
