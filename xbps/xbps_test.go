package xbps

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	require.Equal(t, "foo-1.2.3.beta.1.git.1_2.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameDefaultRelease(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	require.Equal(t, "foo-1.2.3.beta.1.git.1_1.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameNoarch(t *testing.T) {
	info := exampleInfo()
	info.Arch = "all"
	require.Equal(t, "foo-1.2.3.beta.1.git.1_2.noarch.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameGenericARM(t *testing.T) {
	for input, expected := range map[string]string{"arm6": "armv6l", "arm7": "armv7l"} {
		info := exampleInfo()
		info.Arch = input
		require.Equal(t, "foo-1.2.3.beta.1.git.1_2."+expected+".xbps", Default.ConventionalFileName(info))
	}
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
		"armv6l":  "armv6l",
		"arm7":    "armv7l",
		"armv7l":  "armv7l",
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

func TestEnsureValidArchIsIdempotent(t *testing.T) {
	// Package normalizes info.Arch and propsManifest normalizes it again, so
	// every value the map produces must itself be accepted by the map.
	for input, expected := range archToXBPS {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = input
			first, err := ensureValidArch(info)
			require.NoError(t, err)
			require.Equal(t, expected, first.Arch)
			second, err := ensureValidArch(first)
			require.NoError(t, err)
			require.Equal(t, expected, second.Arch)
		})
	}
}

func TestPackageAcceptsEveryMappedArch(t *testing.T) {
	// Regression test: arm6 and arm7 used to fail inside Package because the
	// second normalization in propsManifest did not recognize armv6l/armv7l.
	for input, expected := range archToXBPS {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = input
			var buf bytes.Buffer
			require.NoError(t, Default.Package(info, &buf))

			entries := readTarEntries(t, buf.Bytes())
			require.Contains(t, string(entries["./props.plist"]), "<key>architecture</key><string>"+expected+"</string>")
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
	require.Equal(t, "1.2.3.beta.1.git.1", version(info))

	info.Prerelease = "-rc1-"
	info.VersionMetadata = ".build2."
	require.Equal(t, "1.2.3.rc1.build2", version(info))
}

func TestVersionFoldsReservedSeparatorsInEveryComponent(t *testing.T) {
	testCases := map[string]struct {
		version, prerelease, metadata string
		expected                      string
	}{
		"hyphenated prerelease and metadata": {"v1.2.3", "beta-1", "git-1", "1.2.3.beta.1.git.1"},
		"version_schema none with hyphen":    {"1.2.3-rc1", "", "", "1.2.3.rc1"},
		"underscore in base":                 {"1.2_3", "", "", "1.2.3"},
		"underscore in metadata":             {"1.0", "", "build_7", "1.0.build.7"},
		"repeated separators collapse":       {"1.0", "rc--1", "", "1.0.rc.1"},
		"plain":                              {"1.0.0", "", "", "1.0.0"},
	}
	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			info := exampleInfo()
			info.Version = testCase.version
			info.Prerelease = testCase.prerelease
			info.VersionMetadata = testCase.metadata
			got := version(info)
			require.Equal(t, testCase.expected, got)
			require.NotContains(t, got, "-")
			require.NotContains(t, got, "_")
		})
	}
}

func TestSplitPkgverMirrorsXBPS(t *testing.T) {
	// Expectations follow xbps_pkg_name/xbps_pkg_version in xbps lib/util.c.
	testCases := []struct {
		pkgver         string
		name, ver, rev string
		ok             bool
	}{
		{"foo-1.0_1", "foo", "1.0", "1", true},
		{"foo-bar-1.2.3_2", "foo-bar", "1.2.3", "2", true},
		{"foo-1.2.3.beta.1.git.1_2", "foo", "1.2.3.beta.1.git.1", "2", true},
		{"perl-Digest-1.17_01_1", "perl-Digest", "1.17", "01_1", true},
		{"foo-1.2.3.beta-1.git-1_2", "foo-1.2.3.beta-1.git", "1", "2", true}, // the defect: wrong name
		{"foo", "", "", "", false},
		{"foo-1.0", "", "", "", false},
		{"foo-_1", "", "", "", false},
		{"foo-beta_1", "", "", "", false},
		{"foo-1.0_x", "", "", "", false},
	}
	for _, testCase := range testCases {
		name, ver, rev, ok := splitPkgver(testCase.pkgver)
		require.Equal(t, testCase.ok, ok, testCase.pkgver)
		require.Equal(t, testCase.name, name, testCase.pkgver)
		require.Equal(t, testCase.ver, ver, testCase.pkgver)
		require.Equal(t, testCase.rev, rev, testCase.pkgver)
	}
}

func TestPkgverIsParseableByXBPS(t *testing.T) {
	// Contract test: whatever nFPM produces, xbps must parse back to the
	// configured package name, with no '-' in the version and exactly one '_'
	// separating version from revision. Hyphenated prerelease and metadata
	// values and version_schema=none versions all have to satisfy this.
	testCases := map[string]struct {
		name, version, prerelease, metadata, release string
		expected                                     string
	}{
		"hyphenated prerelease and metadata": {"foo", "v1.2.3", "beta-1", "git-1", "2", "foo-1.2.3.beta.1.git.1_2"},
		"version_schema none":                {"foo", "1.2.3-rc1", "", "", "", "foo-1.2.3.rc1_1"},
		"hyphenated package name":            {"foo-bar", "1.0", "rc-2", "", "3", "foo-bar-1.0.rc.2_3"},
		"name ending in digits":              {"libfoo-2", "2.4", "", "", "", "libfoo-2-2.4_1"},
	}
	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			info := exampleInfo()
			info.Name = testCase.name
			info.Version = testCase.version
			info.Prerelease = testCase.prerelease
			info.VersionMetadata = testCase.metadata
			info.Release = testCase.release

			pv, err := pkgver(info)
			require.NoError(t, err)
			require.Equal(t, testCase.expected, pv)

			parsedName, parsedVersion, parsedRevision, ok := splitPkgver(pv)
			require.True(t, ok, "xbps cannot parse %q", pv)
			require.Equal(t, info.Name, parsedName)
			require.Equal(t, version(info), parsedVersion)
			require.NotContains(t, parsedVersion, "-")
			require.Equal(t, 1, strings.Count(strings.TrimPrefix(pv, info.Name), "_"))
			require.Equal(t, revisionOrDefault(info), parsedRevision)
		})
	}
}

func revisionOrDefault(info *nfpm.Info) string {
	if strings.TrimSpace(info.Release) == "" {
		return "1"
	}
	return strings.TrimSpace(info.Release)
}

func TestPkgverRejectsVersionWithoutDigit(t *testing.T) {
	info := exampleInfo()
	info.Version = "latest"
	info.Prerelease = ""
	info.VersionMetadata = ""
	_, err := pkgver(info)
	require.ErrorContains(t, err, "not parseable by xbps")

	err = Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, "not parseable by xbps")
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
	require.Contains(t, string(entries["./props.plist"]), "foo-1.2.3.beta.1.git.1_2")
	require.Contains(t, string(entries["./props.plist"]), "Foo does things")
	require.Contains(t, string(entries["./files.plist"]), "/etc/fake/fake.conf")
}

func TestPackageControlEntriesComeFirst(t *testing.T) {
	info := exampleInfoWithoutExplicitDirs()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	names := readTarNames(t, buf.Bytes())
	require.GreaterOrEqual(t, len(names), 4)
	require.Equal(t, []string{"./props.plist", "./files.plist"}, names[:2])
	payload := append([]string(nil), names[2:]...)
	require.True(t, sort.StringsAreSorted(payload), "payload entries should be sorted after control entries: %v", payload)
}

func TestPackageControlEntriesFollowNativeOrder(t *testing.T) {
	// xbps reads at most four leading entries and stops at files.plist, both
	// when unpacking (lib/package_unpack.c) and when internalizing scripts
	// (lib/transaction_internalize.c). INSTALL and REMOVE must therefore come
	// before the plists, exactly as xbps-create writes them; anything after
	// files.plist is extracted into the root filesystem as payload.
	info := exampleInfo()
	dir := t.TempDir()
	info.Scripts.PreInstall = writeTempScript(t, dir, "preinstall.sh", "echo preinstall\n")
	info.Scripts.PostRemove = writeTempScript(t, dir, "postremove.sh", "echo postremove\n")

	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	names := readTarNames(t, buf.Bytes())
	require.GreaterOrEqual(t, len(names), 5)
	require.Equal(t, []string{"./INSTALL", "./REMOVE", "./props.plist", "./files.plist"}, names[:4])
	for _, name := range names[4:] {
		require.NotContains(t, []string{"./INSTALL", "./REMOVE", "./props.plist", "./files.plist"}, name)
	}
	require.True(t, sort.StringsAreSorted(names[4:]), "payload entries should be sorted after control entries: %v", names[4:])
}

func TestPackageControlEntriesOmitAbsentScripts(t *testing.T) {
	info := exampleInfoWithoutExplicitDirs()
	dir := t.TempDir()
	info.Scripts.PostRemove = writeTempScript(t, dir, "postremove.sh", "echo postremove\n")

	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	names := readTarNames(t, buf.Bytes())
	require.Equal(t, []string{"./REMOVE", "./props.plist", "./files.plist"}, names[:3])
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

	// Every hook is called with the full native argument list.
	require.Contains(t, install, `preinstall "$@"`)
	require.Contains(t, install, `postinstall "$@"`)
	require.Contains(t, remove, `preremove "$@"`)
	require.Contains(t, remove, `postremove "$@"`)
}

// exampleInfoWithoutExplicitDirs drops the explicit "dir" entry so that no
// generated INSTALL/REMOVE scripts are needed.
func exampleInfoWithoutExplicitDirs() *nfpm.Info {
	info := exampleInfo()
	var contents []*files.Content
	for _, content := range info.Contents {
		if content.Type != files.TypeDir {
			contents = append(contents, content)
		}
	}
	info.Contents = contents
	return info
}

func TestPackageOmitsLifecycleScriptsWhenUnset(t *testing.T) {
	info := exampleInfoWithoutExplicitDirs()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	entries := readTarEntries(t, buf.Bytes())
	require.NotContains(t, entries, "./INSTALL")
	require.NotContains(t, entries, "./REMOVE")
}

func requireShell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell required")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found in PATH")
	}
	return sh
}

// runGeneratedScript executes a generated INSTALL/REMOVE script the way xbps
// does: "sh SCRIPT ACTION PKGNAME VERSION UPDATE CONF ARCH" with the package
// root as the working directory (xbps lib/package_script.c).
func runGeneratedScript(t *testing.T, rootdir string, script []byte, args ...string) (string, string) {
	t.Helper()
	sh := requireShell(t)
	path := filepath.Join(t.TempDir(), "script")
	require.NoError(t, os.WriteFile(path, script, 0o755))
	cmd := exec.Command(sh, append([]string{path}, args...)...)
	cmd.Dir = rootdir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), "stdout: %s\nstderr: %s", stdout.String(), stderr.String())
	return stdout.String(), stderr.String()
}

func TestLifecycleScriptsForwardXBPSArguments(t *testing.T) {
	// xbps passes ACTION PKGNAME VERSION UPDATE CONF ARCH; a hook that uses $4
	// to tell an upgrade from a first install must see the real value, and an
	// argument containing spaces must arrive as a single word.
	info := exampleInfoWithoutExplicitDirs()
	dir := t.TempDir()
	body := "printf '%s|%s|%s|%s|%s|%s|%s\\n' \"$#\" \"$1\" \"$2\" \"$3\" \"$4\" \"$5\" \"$6\"\n"
	info.Scripts.PreInstall = writeTempScript(t, dir, "preinstall.sh", body)
	info.Scripts.PostInstall = writeTempScript(t, dir, "postinstall.sh", body)
	info.Scripts.PreRemove = writeTempScript(t, dir, "preremove.sh", body)
	info.Scripts.PostRemove = writeTempScript(t, dir, "postremove.sh", body)

	install, err := renderInstallScript(info)
	require.NoError(t, err)
	remove, err := renderRemoveScript(info)
	require.NoError(t, err)

	for _, script := range [][]byte{install, remove} {
		for _, action := range []string{"pre", "post"} {
			stdout, _ := runGeneratedScript(t, t.TempDir(), script, action, "foo", "1.2.3_2", "yes", "no", "x86 64")
			require.Equal(t, "6|"+action+"|foo|1.2.3_2|yes|no|x86 64\n", stdout)
		}
	}
}

func TestInstallScriptCreatesExplicitDirectories(t *testing.T) {
	// xbps's unpacker skips directory entries in the archive, so explicit
	// "dir" contents are created by the generated INSTALL script at
	// post-install with the requested mode, and removed at post-remove. This
	// mirrors xbps-src's make_dirs/mkdirs trigger.
	info := exampleInfoWithoutExplicitDirs()
	info.Contents = append(info.Contents,
		&files.Content{
			Destination: "/var/lib/foo",
			Type:        files.TypeDir,
			FileInfo:    &files.ContentFileInfo{Mode: 0o750 | os.ModeDir, Owner: "root", Group: "root"},
		},
		&files.Content{
			Destination: "/srv/foo's cache",
			Type:        files.TypeDir,
			FileInfo:    &files.ContentFileInfo{Mode: 0o700 | os.ModeDir},
		},
	)
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	install, err := renderInstallScript(info)
	require.NoError(t, err)
	remove, err := renderRemoveScript(info)
	require.NoError(t, err)

	require.Contains(t, string(install), "nfpm_make_dir '/srv/foo'\\''s cache' 0700 'root' 'root'")
	require.Contains(t, string(install), "nfpm_make_dir '/var/lib/foo' 0750 'root' 'root'")
	require.NotContains(t, string(install), "'/var/lib'", "implicit parent directories must not be created explicitly")
	require.NotContains(t, string(install), "'/usr/bin'", "implicit parent directories must not be created explicitly")

	rootdir := t.TempDir()
	_, stderr := runGeneratedScript(t, rootdir, install, "post", "foo", "1.2.3_2", "no", "no", "x86_64")
	require.NotContains(t, stderr, "failed to create")
	require.NotContains(t, stderr, "failed to set mode")

	for path, mode := range map[string]os.FileMode{
		"var/lib/foo":     0o750,
		"srv/foo's cache": 0o700,
	} {
		st, err := os.Stat(filepath.Join(rootdir, path))
		require.NoError(t, err, path)
		require.True(t, st.IsDir(), path)
		require.Equal(t, mode, st.Mode().Perm(), path)
	}

	// pre-install must not touch the filesystem, and re-running post-install
	// must be idempotent.
	_, _ = runGeneratedScript(t, rootdir, install, "post", "foo", "1.2.3_2", "yes", "no", "x86_64")
	_, _ = runGeneratedScript(t, rootdir, remove, "post", "foo", "1.2.3_2", "no", "no", "x86_64")
	for _, path := range []string{"var/lib/foo", "srv/foo's cache"} {
		_, err := os.Stat(filepath.Join(rootdir, path))
		require.ErrorIs(t, err, os.ErrNotExist, path)
	}

	// A directory that is no longer empty is left alone.
	require.NoError(t, os.MkdirAll(filepath.Join(rootdir, "var/lib/foo"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(rootdir, "var/lib/foo/state"), []byte("x"), 0o600))
	_, _ = runGeneratedScript(t, rootdir, remove, "post", "foo", "1.2.3_2", "no", "no", "x86_64")
	_, err = os.Stat(filepath.Join(rootdir, "var/lib/foo/state"))
	require.NoError(t, err)
}

func TestDirModeHonoursBothSpecialBitEncodings(t *testing.T) {
	testCases := map[string]struct {
		mode     os.FileMode
		expected string
	}{
		"plain":               {0o755, "0755"},
		"yaml octal setgid":   {0o2775, "2775"},
		"yaml octal sticky":   {0o1777, "1777"},
		"go setgid":           {os.ModeDir | os.ModeSetgid | 0o775, "2775"},
		"go setuid and stick": {os.ModeSetuid | os.ModeSticky | 0o755, "5755"},
	}
	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			content := &files.Content{
				Destination: "/var/lib/foo",
				Type:        files.TypeDir,
				FileInfo:    &files.ContentFileInfo{Mode: testCase.mode},
			}
			require.Equal(t, testCase.expected, dirMode(content))
		})
	}
}

func TestRemoveScriptToleratesSetEAndNonEmptyDirectory(t *testing.T) {
	// A user hook that enables "set -e" affects the whole generated script;
	// the directory cleanup must still exit 0 when a directory is not empty.
	info := exampleInfo()
	dir := t.TempDir()
	info.Scripts.PostRemove = writeTempScript(t, dir, "postremove.sh", "set -e\necho postremove\n")
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	remove, err := renderRemoveScript(info)
	require.NoError(t, err)

	rootdir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(rootdir, "var/lib/foo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(rootdir, "var/lib/foo/state"), []byte("x"), 0o600))

	stdout, _ := runGeneratedScript(t, rootdir, remove, "post", "foo", "1.2.3_2", "no", "no", "x86_64")
	require.Equal(t, "postremove\n", stdout)
	_, err = os.Stat(filepath.Join(rootdir, "var/lib/foo/state"))
	require.NoError(t, err)
}

func TestInstallScriptRunsDirectoryCreationBeforeUserHook(t *testing.T) {
	info := exampleInfo()
	dir := t.TempDir()
	info.Scripts.PostInstall = writeTempScript(t, dir, "postinstall.sh", "test -d ./var/lib/foo && echo dir-ready\n")
	info.Scripts.PostRemove = writeTempScript(t, dir, "postremove.sh", "test -d ./var/lib/foo && echo dir-still-present\n")
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	install, err := renderInstallScript(info)
	require.NoError(t, err)
	remove, err := renderRemoveScript(info)
	require.NoError(t, err)

	rootdir := t.TempDir()
	stdout, _ := runGeneratedScript(t, rootdir, install, "post", "foo", "1.2.3_2", "no", "no", "x86_64")
	require.Equal(t, "dir-ready\n", stdout)

	// The user's post-remove hook runs before the directories are removed.
	stdout, _ = runGeneratedScript(t, rootdir, remove, "post", "foo", "1.2.3_2", "no", "no", "x86_64")
	require.Equal(t, "dir-still-present\n", stdout)
	_, err = os.Stat(filepath.Join(rootdir, "var/lib/foo"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPackageEmitsDirectoryScriptsWithoutUserHooks(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	entries := readTarEntries(t, buf.Bytes())
	require.Contains(t, string(entries["./INSTALL"]), "nfpm_make_dir '/var/lib/foo' 0755 'root' 'root'")
	require.Contains(t, string(entries["./REMOVE"]), "rmdir .'/var/lib/foo'")
	require.NotContains(t, string(entries["./INSTALL"]), "pre)")
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

func TestPackageRemovesStaleSignatureSidecar(t *testing.T) {
	// internal/cmd/package.go removes only the package file when a build
	// fails, so a sidecar from an earlier signed build must not survive an
	// unsigned rebuild, a failed rebuild, or a signed rebuild of different
	// bytes.
	t.Run("unsigned rebuild", func(t *testing.T) {
		info := exampleInfo()
		target := filepath.Join(t.TempDir(), Default.ConventionalFileName(info))
		require.NoError(t, os.WriteFile(target+".sig2", []byte("stale"), 0o644))

		info.Target = target
		f, err := os.Create(target)
		require.NoError(t, err)
		require.NoError(t, Default.Package(info, f))
		require.NoError(t, f.Close())

		_, err = os.Stat(target + ".sig2")
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("failed rebuild", func(t *testing.T) {
		info := exampleInfo()
		info.Scripts.PreInstall = filepath.Join(t.TempDir(), "missing-preinstall.sh")
		target := filepath.Join(t.TempDir(), Default.ConventionalFileName(info))
		require.NoError(t, os.WriteFile(target+".sig2", []byte("stale"), 0o644))

		info.Target = target
		require.Error(t, Default.Package(info, &bytes.Buffer{}))

		_, err := os.Stat(target + ".sig2")
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("signed rebuild replaces sidecar", func(t *testing.T) {
		info := exampleInfo()
		info.XBPS.Signature.KeyFile = "../internal/sign/testdata/rsa_unprotected.priv"
		target := filepath.Join(t.TempDir(), Default.ConventionalFileName(info))
		require.NoError(t, os.WriteFile(target+".sig2", []byte("stale"), 0o644))

		info.Target = target
		f, err := os.Create(target)
		require.NoError(t, err)
		require.NoError(t, Default.Package(info, f))
		require.NoError(t, f.Close())

		sidecar, err := os.ReadFile(target + ".sig2")
		require.NoError(t, err)
		require.NotEqual(t, "stale", string(sidecar))
		requireSignatureSidecarVerifies(t, target, "../internal/sign/testdata/rsa_unprotected.pub")
	})

	t.Run("no target leaves nothing to remove", func(t *testing.T) {
		info := exampleInfo()
		require.NoError(t, Default.Package(info, &bytes.Buffer{}))
	})
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
	require.Equal(t, "foo-1.2.3.beta.1.git.1_2", props["pkgver"])
	require.Equal(t, "1.2.3.beta.1.git.1", props["version"])
	require.Equal(t, "Foo does things", props["short_desc"])
	require.Equal(t, "Foo does things\nAnd does them well.", props["long_desc"])
	require.Equal(t, "https://example.com/foo", props["homepage"])
	require.Equal(t, "MIT", props["license"])
	require.Equal(t, "Carlos A Becker <pkg@carlosbecker.com>", props["maintainer"])
	require.Equal(t, plistArray{"bash>=0", "zlib>=1.0_1"}, props["run_depends"])
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

func TestRunDependencyProducesXBPSResolvableForms(t *testing.T) {
	// xbps resolves run_depends with xbps_pkgpattern_name() or, failing that,
	// xbps_pkg_name() (lib/transaction_pkg_deps.c); a bare name satisfies
	// neither and aborts the transaction with "can't guess pkgname".
	testCases := map[string]string{
		"bash":                  "bash>=0",
		"ca-certificates":       "ca-certificates>=0",
		"libfoo-2":              "libfoo-2>=0",
		"zlib>=1.0_1":           "zlib>=1.0_1",
		"bar<2.0":               "bar<2.0",
		"foo>=1.0<2.0":          "foo>=1.0<2.0",
		"foo-[0-9]*":            "foo-[0-9]*",
		"foo-1.0_1":             "foo-1.0_1",
		"perl-Digest-1.17_01_1": "perl-Digest-1.17_01_1",
		"  bash  ":              "bash>=0",
	}
	for input, expected := range testCases {
		got, err := runDependency(input)
		require.NoError(t, err, input)
		require.Equal(t, expected, got, input)
		// Mirror xbps_pkgpattern_name: the name must be recoverable.
		idx := strings.IndexAny(got, patternChars)
		if idx >= 0 {
			require.NotEmpty(t, strings.TrimSuffix(got[:idx], "-"))
		} else {
			_, _, _, ok := splitPkgver(got)
			require.True(t, ok, got)
		}
	}
}

func TestRunDependencyRejectsForeignSyntax(t *testing.T) {
	for _, input := range []string{"", "   ", "foo (>= 1.0)", "foo >= 1.0", ">=1.0", "->=1.0"} {
		_, err := runDependency(input)
		require.Error(t, err, "%q", input)
		require.ErrorContains(t, err, "xbps:")
	}
}

func TestPropsManifestRejectsForeignDependencySyntax(t *testing.T) {
	info := exampleInfo()
	info.Depends = []string{"foo (>= 1.0)"}
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	_, err := propsManifest(info)
	require.ErrorContains(t, err, `dependency "foo (>= 1.0)" must use xbps syntax`)

	err = Default.Package(exampleInfoWithDepends("foo >= 1.0"), &bytes.Buffer{})
	require.ErrorContains(t, err, "must use xbps syntax")
}

func exampleInfoWithDepends(depends ...string) *nfpm.Info {
	info := exampleInfo()
	info.Depends = depends
	return info
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

func TestWritePlistValueScalarTypes(t *testing.T) {
	var buf bytes.Buffer
	root := plistDict{
		"flagTrue":  true,
		"flagFalse": false,
		"count":     int64(7),
		"size":      uint64(42),
		"list":      plistArray{int64(1), "two"},
	}
	require.NoError(t, writePlistValue(&buf, root))
	out := buf.String()
	require.Contains(t, out, "<true/>")
	require.Contains(t, out, "<false/>")
	require.Contains(t, out, "<integer>7</integer>")
	require.Contains(t, out, "<integer>42</integer>")
	require.Contains(t, out, "<array><integer>1</integer><string>two</string></array>")
}

func TestWritePlistValueUnsupportedType(t *testing.T) {
	var buf bytes.Buffer
	require.ErrorContains(t, writePlistValue(&buf, 3.14), "unsupported plist value type")
}

func TestWritePlistValueErrorPropagatesThroughContainers(t *testing.T) {
	var buf bytes.Buffer
	require.ErrorContains(t, writePlistValue(&buf, plistArray{3.14}), "unsupported plist value type")

	buf.Reset()
	require.ErrorContains(t, writePlistValue(&buf, plistDict{"bad": 3.14}), "unsupported plist value type")
}

func TestFileEntryDirectoryOmitsChecksum(t *testing.T) {
	entry, err := fileEntry(&files.Content{
		Destination: "/var/lib/foo",
		Type:        files.TypeDir,
	})
	require.NoError(t, err)
	require.Equal(t, "/var/lib/foo", entry["file"])
	require.NotContains(t, entry, "sha256")
	require.NotContains(t, entry, "size")
}

func TestFileEntryReturnsOpenError(t *testing.T) {
	_, err := fileEntry(&files.Content{
		Source:      filepath.Join(t.TempDir(), "missing"),
		Destination: "/usr/bin/foo",
	})
	require.Error(t, err)
}

func TestWriteContentEntryDirectory(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, writeContentEntry(tw, &files.Content{
		Destination: "/var/lib/foo",
		Type:        files.TypeDir,
		FileInfo:    &files.ContentFileInfo{MTime: testMTime},
	}))
	require.NoError(t, tw.Close())

	hdr, err := tar.NewReader(&buf).Next()
	require.NoError(t, err)
	require.Equal(t, byte(tar.TypeDir), hdr.Typeflag)
}

func TestWriteContentEntryReturnsOpenError(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := writeContentEntry(tw, &files.Content{
		Source:      filepath.Join(t.TempDir(), "missing"),
		Destination: "/usr/bin/foo",
		FileInfo:    &files.ContentFileInfo{MTime: testMTime},
	})
	require.Error(t, err)
}
