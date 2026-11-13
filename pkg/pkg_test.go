package pkg_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/macostest"
	"github.com/goreleaser/nfpm/v2/pkg"
	"go.digitalxero.dev/osx/cms"
	osxpkg "go.digitalxero.dev/osx/pkg"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identifier = "com.example.foo"

// exampleInfo is the smallest useful package: one program.
func exampleInfo(t *testing.T) *nfpm.Info {
	t.Helper()
	return nfpm.WithDefaults(&nfpm.Info{
		Name:    "foo",
		Arch:    "arm64",
		Version: "1.2.3",
		MTime:   macostest.MTime,
		Overridables: nfpm.Overridables{
			Contents: files.Contents{
				{
					Source:      macostest.WriteFile(t, "foo", "#!/bin/sh\necho foo\n"),
					Destination: "/usr/local/bin/foo",
					FileInfo:    &files.ContentFileInfo{Mode: 0o755},
				},
			},
			Pkg: nfpm.Pkg{Identifier: identifier},
		},
	})
}

func build(t *testing.T, info *nfpm.Info) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, pkg.Default.Package(info, &out))
	return out.Bytes()
}

// open builds the package and reads it back, checking on the way that the
// container, the payload and the bill of materials agree with one another.
func open(t *testing.T, info *nfpm.Info) osxpkg.Archive {
	t.Helper()
	raw := build(t, info)
	archive, err := osxpkg.Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	require.NoError(t, archive.Verify())
	require.Len(t, archive.Components(), 1)
	require.NoError(t, archive.Components()[0].Verify())
	return archive
}

func payload(t *testing.T, archive osxpkg.Archive) osxpkg.Listing[osxpkg.PayloadEntry] {
	t.Helper()
	listing, err := archive.Components()[0].Payload()
	require.NoError(t, err)
	return listing
}

func content(t *testing.T, entry osxpkg.PayloadEntry) string {
	t.Helper()
	reader, err := entry.Open()
	require.NoError(t, err)
	defer reader.Close() // nolint: errcheck
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	return string(body)
}

func packageError(t *testing.T, info *nfpm.Info) error {
	t.Helper()
	var out bytes.Buffer
	err := pkg.Default.Package(info, &out)
	require.Error(t, err)
	return err
}

func TestRegistered(t *testing.T) {
	packager, err := nfpm.Get("pkg")
	require.NoError(t, err)
	require.Same(t, pkg.Default, packager)
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".pkg", pkg.Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo(t)
	require.Equal(t, "foo_1.2.3_arm64.pkg", pkg.Default.ConventionalFileName(info))

	info.Arch = "amd64"
	require.Equal(t, "foo_1.2.3_x86_64.pkg", pkg.Default.ConventionalFileName(info))

	info.Pkg.Arch = "universal"
	require.Equal(t, "foo_1.2.3_universal.pkg", pkg.Default.ConventionalFileName(info))
	require.Equal(t, "amd64", info.Arch, "naming a package must not change the info")

	// Packaging reports an unsupported architecture; naming must not hide it.
	info.Pkg.Arch = ""
	info.Arch = "riscv64"
	require.Equal(t, "foo_1.2.3_riscv64.pkg", pkg.Default.ConventionalFileName(info))
}

// Releases that differ in any part of their version must not overwrite one
// another when packaged into the same directory.
func TestConventionalFileNameKeepsReleaseIdentity(t *testing.T) {
	names := map[string]string{}
	for _, version := range []string{"1.2.3", "1.2.3-rc1", "1.2.3-rc2", "1.2.3+build5"} {
		info := exampleInfo(t)
		info.Version = version
		info.Prerelease, info.VersionMetadata = "", ""
		nfpm.WithDefaults(info)
		name := pkg.Default.ConventionalFileName(info)
		require.NotContains(t, names, name, "%s and %s share a file name", version, names[name])
		names[name] = version
	}

	info := exampleInfo(t)
	info.Release = "2"
	require.Equal(t, "foo_1.2.3-2_arm64.pkg", pkg.Default.ConventionalFileName(info))
}

func TestProductArchive(t *testing.T) {
	info := exampleInfo(t)
	info.Pkg.MinOSVersion = "13.0"
	archive := open(t, info)

	require.True(t, archive.IsProduct(), "the package must carry a Distribution, the only place the architecture is declared")
	assert.Equal(t, "foo", archive.Title())
	assert.Equal(t, identifier, archive.ProductIdentifier())
	assert.Equal(t, "1.2.3", archive.ProductVersion())
	assert.Equal(t, []string{"arm64"}, archive.HostArchitectures())
	assert.Equal(t, "13.0", archive.MinOSVersion())
	assert.False(t, archive.IsSigned())

	component := archive.Components()[0]
	assert.Equal(t, identifier, component.Identifier())
	assert.Equal(t, "1.2.3", component.Version())
	assert.Equal(t, "/", component.InstallLocation())
}

func TestNoMinOSVersionByDefault(t *testing.T) {
	require.Empty(t, open(t, exampleInfo(t)).MinOSVersion())
}

func TestArchitectures(t *testing.T) {
	for arch, want := range map[string][]string{
		"amd64":     {"x86_64"},
		"x86_64":    {"x86_64"},
		"arm64":     {"arm64"},
		"aarch64":   {"arm64"},
		"ARM64":     {"arm64"},
		"all":       {"arm64", "x86_64"},
		"universal": {"arm64", "x86_64"},
	} {
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo(t)
			info.Arch = arch
			require.ElementsMatch(t, want, open(t, info).HostArchitectures())
		})
	}
}

func TestArchOverride(t *testing.T) {
	info := exampleInfo(t)
	info.Arch = "arm64"
	info.Pkg.Arch = "x86_64"
	require.Equal(t, []string{"x86_64"}, open(t, info).HostArchitectures())

	info.Pkg.Arch = "universal"
	require.ElementsMatch(t, []string{"arm64", "x86_64"}, open(t, info).HostArchitectures())
}

// macOS runs on two architectures. Anything else must fail instead of
// producing a package that claims one of them.
func TestUnsupportedArch(t *testing.T) {
	for _, arch := range []string{"386", "arm", "arm7", "riscv64", "ppc64le"} {
		info := exampleInfo(t)
		info.Arch = arch
		require.ErrorContains(t, packageError(t, info), `package arch "`+arch+`" is not supported by macOS`)
	}
	for _, arch := range []string{"amd64", "aarch64", "ARM64", "x64"} {
		info := exampleInfo(t)
		info.Pkg.Arch = arch
		require.ErrorContains(t, packageError(t, info), `package pkg.arch "`+arch+`" is not supported`)
	}
}

func TestIdentifier(t *testing.T) {
	info := exampleInfo(t)
	info.Pkg.Identifier = ""
	require.ErrorContains(t, packageError(t, info), "package pkg.identifier must be provided")

	for _, invalid := range []string{"com.example.foo bar", ".com.example", "com/example", "cömpany.foo"} {
		info := exampleInfo(t)
		info.Pkg.Identifier = invalid
		require.ErrorContains(t, packageError(t, info), "package pkg.identifier", invalid)
	}

	for _, valid := range []string{"foo", "com.example.foo-bar", "com.example.foo_bar", "1password.app"} {
		info := exampleInfo(t)
		info.Pkg.Identifier = valid
		require.Equal(t, valid, open(t, info).Components()[0].Identifier())
	}
}

// The identifier is the upgrade identity: it must not depend on the version
// or the architecture.
func TestIdentifierIsStableAcrossReleases(t *testing.T) {
	first := exampleInfo(t)
	second := exampleInfo(t)
	second.Version = "2.0.0"
	second.Arch = "amd64"
	require.Equal(t,
		open(t, first).Components()[0].Identifier(),
		open(t, second).Components()[0].Identifier())
}

func TestMinOSVersion(t *testing.T) {
	for _, invalid := range []string{"13.x", "v13", "13.0.0.1", "13,0", " 13"} {
		info := exampleInfo(t)
		info.Pkg.MinOSVersion = invalid
		require.ErrorContains(t, packageError(t, info), "package pkg.min_os_version", invalid)
	}
	for _, valid := range []string{"11", "11.0", "13.3.1"} {
		info := exampleInfo(t)
		info.Pkg.MinOSVersion = valid
		require.Equal(t, valid, open(t, info).MinOSVersion())
	}
}

func TestVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*nfpm.Info)
		want string
	}{
		{"plain", func(*nfpm.Info) {}, "1.2.3"},
		{"prerelease", func(i *nfpm.Info) { i.Prerelease = "rc1" }, "1.2.3-rc1"},
		{"metadata", func(i *nfpm.Info) { i.VersionMetadata = "git" }, "1.2.3+git"},
		{"release", func(i *nfpm.Info) { i.Release = "4" }, "1.2.3-4"},
		{"everything", func(i *nfpm.Info) {
			i.Prerelease, i.VersionMetadata, i.Release = "rc1", "git", "4"
		}, "1.2.3-rc1+git-4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := exampleInfo(t)
			tc.edit(info)
			archive := open(t, info)
			assert.Equal(t, tc.want, archive.ProductVersion())
			assert.Equal(t, tc.want, archive.Components()[0].Version())
		})
	}
}

func TestContents(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      macostest.WriteFile(t, "app.conf", "key=value\n"),
			Destination: "/etc/foo/app.conf",
			Type:        files.TypeConfig,
			FileInfo:    &files.ContentFileInfo{Mode: 0o640, Group: "admin"},
		},
		&files.Content{
			Source:      "/usr/local/bin/foo",
			Destination: "/usr/local/bin/foo-link",
			Type:        files.TypeSymlink,
		},
		&files.Content{
			Destination: "/Library/Application Support/Foo",
			Type:        files.TypeDir,
			FileInfo:    &files.ContentFileInfo{Mode: 0o750, Owner: "daemon", Group: "staff"},
		},
		&files.Content{
			Source:      macostest.WriteFile(t, "helper", "helper\n"),
			Destination: "/usr/local/libexec/foo-helper",
			FileInfo:    &files.ContentFileInfo{Mode: 0o755 | fs.ModeSetuid, Owner: "501", Group: "20"},
		},
		&files.Content{
			Source:      macostest.WriteFile(t, "unicode", "héllo\n"),
			Destination: "/Library/Application Support/Foo/héllo-世界.txt",
			FileInfo:    &files.ContentFileInfo{Mode: 0o644},
		},
	)
	entries := payload(t, open(t, info))

	program, err := entries.Entry("/usr/local/bin/foo")
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho foo\n", content(t, program))
	assert.Equal(t, fs.FileMode(0o755), program.Mode())
	assert.Zero(t, program.UID())
	assert.Zero(t, program.GID(), "nfpm's default group root is wheel, group 0, on macOS")
	assert.True(t, macostest.MTime.Equal(program.ModTime()), program.ModTime())

	// /etc is a symbolic link on macOS; the entry is stored below the
	// directory it points to and no "etc" entry exists.
	config, err := entries.Entry("/private/etc/foo/app.conf")
	require.NoError(t, err)
	assert.Equal(t, "key=value\n", content(t, config))
	assert.Equal(t, fs.FileMode(0o640), config.Mode())
	assert.Equal(t, uint32(80), config.GID())
	_, err = entries.Entry("/etc")
	require.ErrorIs(t, err, osxpkg.ErrNotExist)

	link, err := entries.Entry("/usr/local/bin/foo-link")
	require.NoError(t, err)
	assert.Equal(t, fs.ModeSymlink, link.Mode().Type())
	assert.Equal(t, "/usr/local/bin/foo", link.LinkTarget())

	dir, err := entries.Entry("/Library/Application Support/Foo")
	require.NoError(t, err)
	assert.Equal(t, fs.ModeDir|0o750, dir.Mode())
	assert.Equal(t, uint32(1), dir.UID())
	assert.Equal(t, uint32(20), dir.GID())

	helper, err := entries.Entry("/usr/local/libexec/foo-helper")
	require.NoError(t, err)
	assert.Equal(t, 0o755|fs.ModeSetuid, helper.Mode())
	assert.Equal(t, uint32(501), helper.UID())
	assert.Equal(t, uint32(20), helper.GID())

	unicode, err := entries.Entry("/Library/Application Support/Foo/héllo-世界.txt")
	require.NoError(t, err)
	assert.Equal(t, "héllo\n", content(t, unicode))
}

func TestContentsForOtherPackagersAreLeftOut(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = append(info.Contents, &files.Content{
		Source:      macostest.WriteFile(t, "linux", "linux\n"),
		Destination: "/usr/bin/foo",
		Packager:    "deb",
	})
	entries := payload(t, open(t, info))
	_, err := entries.Entry("/usr/bin/foo")
	require.ErrorIs(t, err, osxpkg.ErrNotExist)
}

func TestNonASCIIName(t *testing.T) {
	info := exampleInfo(t)
	info.Name = "Café 世界"
	archive := open(t, info)
	require.Equal(t, "Café 世界", archive.Title())
	require.Equal(t, "Café 世界_1.2.3_arm64.pkg", pkg.Default.ConventionalFileName(info))
}

func TestEmptyContents(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = nil
	require.ErrorContains(t, packageError(t, info), "package contents must include at least one")
}

func TestMissingSource(t *testing.T) {
	info := exampleInfo(t)
	info.DisableGlobbing = true
	info.Contents[0].Source = filepath.Join(t.TempDir(), "missing")
	require.Error(t, packageError(t, info))
}

// macOS refuses to install outside a handful of locations; a package that
// tries must fail when it is built, where the reason can still be given.
func TestProtectedDestinations(t *testing.T) {
	for _, destination := range []string{
		"/usr/bin/foo",
		"/usr/lib/libfoo.dylib",
		"/bin/foo",
		"/sbin/foo",
		"/System/Library/foo",
		"/foo/bar",
		"/foo",
		"/usrlocal/foo",
		"/library/foo",
	} {
		info := exampleInfo(t)
		info.Contents[0].Destination = destination
		require.ErrorContains(t, packageError(t, info),
			`package contents "`+destination+`": macOS does not allow installing there`)
	}
	for _, destination := range []string{
		"/Applications/Foo.app/Contents/MacOS/foo",
		"/Library/Foo/foo",
		"/opt/foo/bin/foo",
		"/usr/local/bin/foo",
		"/private/var/db/foo",
		"/var/db/foo",
		"/tmp/foo",
		"/Users/Shared/foo",
	} {
		info := exampleInfo(t)
		info.Contents[0].Destination = destination
		require.Len(t, open(t, info).Components(), 1, destination)
	}
}

// The directories macOS provides are not a package's to change: listing one,
// as a tree rooted at / does, must not carry its mode or owner into the
// package.
func TestSystemDirectoriesAreNotClaimed(t *testing.T) {
	for _, destination := range []string{"/usr", "/usr/local", "/Library", "/Applications", "/etc", "/private", "/private/var"} {
		t.Run(destination, func(t *testing.T) {
			info := exampleInfo(t)
			info.Contents = append(info.Contents,
				&files.Content{
					Destination: destination,
					Type:        files.TypeDir,
					FileInfo:    &files.ContentFileInfo{Mode: 0o700, Owner: "501", Group: "20"},
				},
				&files.Content{
					Source:      macostest.WriteFile(t, "child", "child\n"),
					Destination: destination + "/local/foo-child/file",
				},
			)
			if destination == "/usr" {
				// Only /usr/local is open to packages; the child above is.
				info.Contents[2].Destination = "/usr/local/foo-child/file"
			}
			entries := payload(t, open(t, info))
			dir, err := entries.Entry(strings.Replace(destination, "/etc", "/private/etc", 1))
			require.NoError(t, err)
			assert.Equal(t, fs.ModeDir|0o755, dir.Mode())
			assert.Zero(t, dir.UID())
			assert.Zero(t, dir.GID())
		})
	}
}

// They cannot be replaced by a file or a symbolic link either.
func TestSystemDirectoryCannotBeReplaced(t *testing.T) {
	for _, destination := range []string{"/usr/local", "/Library", "/etc", "/private"} {
		info := exampleInfo(t)
		info.Contents = append(info.Contents, &files.Content{
			Source:      "/somewhere/else",
			Destination: destination,
			Type:        files.TypeSymlink,
		})
		require.ErrorContains(t, packageError(t, info), "is a directory of macOS", destination)
	}
}

// A tree laid out like the root of the disk is the usual way to describe a
// package. Its top-level directories are macOS's, its own directories and
// their modes are the package's.
func TestTreeRootedAtSlash(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "Library", "Foo", "empty"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "usr", "local", "bin"), 0o755))
	require.NoError(t, os.Chmod(filepath.Join(root, "Library", "Foo"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "usr", "local", "bin", "foo"), []byte("foo\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "Library", "Foo", "data"), []byte("data\n"), 0o644))

	info := exampleInfo(t)
	info.Contents = files.Contents{{Source: root, Destination: "/", Type: files.TypeTree}}
	entries := payload(t, open(t, info))

	program, err := entries.Entry("/usr/local/bin/foo")
	require.NoError(t, err)
	assert.Equal(t, "foo\n", content(t, program))
	// The directory's mode is the one on disk, less nfpm's default umask.
	// It is read back instead of assumed: Windows has its own idea of modes.
	onDisk, err := os.Stat(filepath.Join(root, "Library", "Foo"))
	require.NoError(t, err)
	own, err := entries.Entry("/Library/Foo")
	require.NoError(t, err)
	assert.Equal(t, fs.ModeDir|(onDisk.Mode().Perm()&^0o002), own.Mode())
	empty, err := entries.Entry("/Library/Foo/empty")
	require.NoError(t, err, "an empty directory in a tree is installed, not dropped")
	assert.True(t, empty.Mode().IsDir())
}

// Directories nobody listed are left to the installer, which only creates the
// missing ones. They are present in the payload as plain root-owned
// directories and never carry settings taken from another entry.
func TestParentDirectoriesAreNotClaimed(t *testing.T) {
	info := exampleInfo(t)
	info.Contents[0].FileInfo.Owner = "501"
	info.Contents[0].FileInfo.Mode = 0o700
	entries := payload(t, open(t, info))
	for _, parent := range []string{"/usr", "/usr/local", "/usr/local/bin"} {
		dir, err := entries.Entry(parent)
		require.NoError(t, err)
		assert.Equal(t, fs.ModeDir|0o755, dir.Mode(), parent)
		assert.Zero(t, dir.UID(), parent)
		assert.Zero(t, dir.GID(), parent)
	}
}

// macOS volumes ignore letter case and Unicode normalization, so these are
// one path there and two entries would fight over it.
func TestCollidingDestinations(t *testing.T) {
	for name, pair := range map[string][2]string{
		"file case":      {"/usr/local/bin/foo", "/usr/local/bin/FOO"},
		"directory case": {"/opt/Foo/a", "/opt/foo/b"},
		"normalization":  {"/opt/foo/café", "/opt/foo/café"},
		"alias and case": {"/etc/foo/A", "/private/etc/foo/a"},
	} {
		t.Run(name, func(t *testing.T) {
			info := exampleInfo(t)
			info.Contents = files.Contents{
				{Source: macostest.WriteFile(t, "a", "a"), Destination: pair[0]},
				{Source: macostest.WriteFile(t, "b", "b"), Destination: pair[1]},
			}
			require.ErrorContains(t, packageError(t, info), "macOS stores them as the same path")
		})
	}
}

func TestSameFileThroughAlias(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = files.Contents{
		{Source: macostest.WriteFile(t, "a", "a"), Destination: "/etc/foo/a"},
		{Source: macostest.WriteFile(t, "b", "b"), Destination: "/private/etc/foo/a"},
	}
	// Both entries name the very same file.
	require.Error(t, packageError(t, info))
}

// An owner that is not a built-in account has a different number on every
// Mac. Guessing would install the file as root without anyone noticing.
func TestUnknownOwner(t *testing.T) {
	info := exampleInfo(t)
	info.Contents[0].FileInfo.Owner = "alice"
	err := packageError(t, info)
	require.ErrorContains(t, err, `owner "alice" is not a built-in macOS account`)
	require.ErrorContains(t, err, "/usr/local/bin/foo")
	require.ErrorIs(t, err, osxpkg.ErrUnknownAccount)

	info = exampleInfo(t)
	info.Contents[0].FileInfo.Group = "developers"
	require.ErrorContains(t, packageError(t, info), `group "developers" is not a built-in macOS account`)
}

// The installer cannot deliver an ID above 65535; such an entry must fail
// instead of being installed under another owner.
func TestOwnerIDOutOfRange(t *testing.T) {
	info := exampleInfo(t)
	info.Contents[0].FileInfo.Owner = "70000"
	require.ErrorIs(t, packageError(t, info), osxpkg.ErrIDRange)
}

func TestScripts(t *testing.T) {
	info := exampleInfo(t)
	info.Scripts.PreInstall = macostest.WriteFile(t, "preinstall", "#!/bin/sh\necho pre\n")
	info.Scripts.PostInstall = macostest.WriteFile(t, "postinstall", "#!/bin/bash\necho post\n")
	component := open(t, info).Components()[0]

	pre, err := component.PreInstallScript()
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho pre\n", content(t, pre))
	assert.Equal(t, fs.FileMode(0o755), pre.Mode().Perm(), "the installer runs the script directly")

	post, err := component.PostInstallScript()
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/bash\necho post\n", content(t, post))
	assert.Equal(t, fs.FileMode(0o755), post.Mode().Perm())
}

func TestNoScriptsByDefault(t *testing.T) {
	component := open(t, exampleInfo(t)).Components()[0]
	_, err := component.PreInstallScript()
	require.ErrorIs(t, err, osxpkg.ErrNoScript)
	_, err = component.PostInstallScript()
	require.ErrorIs(t, err, osxpkg.ErrNoScript)
}

func TestOnlyOneScript(t *testing.T) {
	info := exampleInfo(t)
	info.Scripts.PostInstall = macostest.WriteFile(t, "postinstall", "#!/bin/sh\n")
	component := open(t, info).Components()[0]
	_, err := component.PreInstallScript()
	require.ErrorIs(t, err, osxpkg.ErrNoScript)
	_, err = component.PostInstallScript()
	require.NoError(t, err)
}

func TestScriptWithoutInterpreterLine(t *testing.T) {
	info := exampleInfo(t)
	info.Scripts.PreInstall = macostest.WriteFile(t, "preinstall", "echo pre\n")
	require.ErrorContains(t, packageError(t, info), "scripts.preinstall")

	info = exampleInfo(t)
	info.Scripts.PostInstall = macostest.WriteFile(t, "postinstall", "")
	require.ErrorContains(t, packageError(t, info), "scripts.postinstall")
}

func TestMissingScript(t *testing.T) {
	info := exampleInfo(t)
	info.Scripts.PreInstall = filepath.Join(t.TempDir(), "missing")
	require.ErrorContains(t, packageError(t, info), "reading preinstall script")
}

// macOS has no package removal. The remove scripts are reported, not
// silently dropped, and never end up in the package.
func TestRemoveScriptsAreIgnoredWithWarning(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	info := exampleInfo(t)
	info.Scripts.PreRemove = macostest.WriteFile(t, "preremove", "#!/bin/sh\necho PREREMOVE_BODY\n")
	info.Scripts.PostRemove = macostest.WriteFile(t, "postremove", "#!/bin/sh\necho POSTREMOVE_BODY\n")
	raw := build(t, info)

	assert.Contains(t, logs.String(), "the preremove script is ignored")
	assert.Contains(t, logs.String(), "the postremove script is ignored")

	archive, err := osxpkg.Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	scripts, err := archive.Components()[0].Scripts()
	require.NoError(t, err)
	require.Empty(t, scripts.Entries())
}

func TestUnsupportedSettingsWarn(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	info := exampleInfo(t)
	info.Depends = []string{"bash"}
	info.Recommends = []string{"git"}
	info.Suggests = []string{"zsh"}
	info.Conflicts = []string{"bar"}
	info.Replaces = []string{"baz"}
	info.Provides = []string{"qux"}
	build(t, info)
	for _, name := range []string{"depends", "recommends", "suggests", "conflicts", "replaces", "provides"} {
		assert.Contains(t, logs.String(), name+" is ignored")
	}
}

func TestNoWarningsForAPlainPackage(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	build(t, exampleInfo(t))
	require.Empty(t, logs.String())
}

func TestConfigFilesWarn(t *testing.T) {
	for _, kind := range []string{files.TypeConfig, files.TypeConfigNoReplace, files.TypeConfigMissingOK} {
		t.Run(kind, func(t *testing.T) {
			logs := macostest.CaptureLogs(t)
			info := exampleInfo(t)
			info.Contents = append(info.Contents, &files.Content{
				Source:      macostest.WriteFile(t, "app.conf", "key=value\n"),
				Destination: "/etc/foo/app.conf",
				Type:        kind,
			})
			entries := payload(t, open(t, info))
			assert.Contains(t, logs.String(), "/etc/foo/app.conf is installed as a regular file and replaced on upgrade")
			_, err := entries.Entry("/private/etc/foo/app.conf")
			require.NoError(t, err, "the file is still installed")
		})
	}
}

func TestReproducible(t *testing.T) {
	info := exampleInfo(t)
	info.Scripts.PostInstall = macostest.WriteFile(t, "postinstall", "#!/bin/sh\n")
	first := build(t, info)
	require.Equal(t, first, build(t, info), "the same input must give the same bytes")

	require.NoError(t, os.WriteFile(info.Contents[0].Source, []byte("#!/bin/sh\necho changed\n"), 0o644))
	require.NotEqual(t, first, build(t, info), "changed content must change the package")
}

func TestPackageDoesNotChangeTheInfo(t *testing.T) {
	info := exampleInfo(t)
	info.Arch = "amd64"
	contents := info.Contents
	build(t, info)
	require.Equal(t, "amd64", info.Arch)
	require.Equal(t, contents, info.Contents)
	require.Len(t, info.Contents, 1, "implicit directories must not leak into the caller's contents")
}

func signedInfo(t *testing.T, p12 string) *nfpm.Info {
	t.Helper()
	info := exampleInfo(t)
	info.Pkg.Signature = nfpm.PkgSignature{
		P12File:       p12,
		TimestampURL:  "none",
		KeyPassphrase: macostest.Passphrase,
	}
	return info
}

func TestSigning(t *testing.T) {
	p12, certificate := macostest.Identity(t)
	archive := open(t, signedInfo(t, p12))

	require.True(t, archive.IsSigned())
	require.NotEmpty(t, archive.Certificates())
	require.Equal(t, certificate.Raw, archive.Certificates()[0])

	// The signature must be a real one over this archive, by this identity.
	verifier, err := cms.NewVerifier().WithRoots(certificate.Raw).Build()
	require.NoError(t, err)
	verification, err := verifier.VerifyDetached(archive.SignedData(), archive.Signature())
	require.NoError(t, err)
	require.Equal(t, certificate.Raw, verification.Certificates()[0])
	_, timestamped := verification.Timestamp()
	require.False(t, timestamped, "timestamp_url: none must not contact a timestamp service")

	// And it must not verify against anyone else.
	_, other := macostest.Identity(t)
	stranger, err := cms.NewVerifier().WithRoots(other.Raw).Build()
	require.NoError(t, err)
	_, err = stranger.VerifyDetached(archive.SignedData(), archive.Signature())
	require.ErrorIs(t, err, cms.ErrVerification)
}

func TestSigningWrongPassphrase(t *testing.T) {
	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.Pkg.Signature.KeyPassphrase = "wrong"
	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, packageError(t, info), &signing)
}

func TestSigningMissingP12(t *testing.T) {
	info := signedInfo(t, filepath.Join(t.TempDir(), "missing.p12"))
	err := packageError(t, info)
	require.ErrorContains(t, err, "p12 file not found")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// A timestamp service that fails is a signing failure, and callers can tell.
func TestSigningTimestampFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.Pkg.Signature.TimestampURL = server.URL
	err := packageError(t, info)

	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &signing)
	require.ErrorIs(t, err, cms.ErrTimestamp)
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// The default is Apple's timestamp service, as productsign does. The request
// is intercepted, so the test proves where it would go without going there.
func TestSigningUsesApplesTimestampServiceByDefault(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	var requested string
	http.DefaultTransport = roundTripper(func(request *http.Request) (*http.Response, error) {
		requested = request.URL.String()
		return nil, errors.New("intercepted")
	})

	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.Pkg.Signature.TimestampURL = ""
	err := packageError(t, info)

	require.ErrorIs(t, err, cms.ErrTimestamp)
	require.Equal(t, cms.DefaultTimestampURL, requested)
	require.Contains(t, requested, "timestamp.apple.com")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// A package that cannot be written is not a signing problem.
func TestWriteFailureIsNotASigningFailure(t *testing.T) {
	p12, _ := macostest.Identity(t)
	err := pkg.Default.Package(signedInfo(t, p12), failingWriter{})
	require.ErrorContains(t, err, "disk full")
	var signing *nfpm.ErrSigningFailure
	require.NotErrorAs(t, err, &signing)

	err = pkg.Default.Package(exampleInfo(t), failingWriter{})
	require.ErrorContains(t, err, "disk full")
	require.NotErrorAs(t, err, &signing)
}

func TestCertificateKindWarning(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	p12, _ := macostest.Identity(t)
	build(t, signedInfo(t, p12))
	require.Contains(t, logs.String(), "is not a Developer ID Installer certificate")

	// A Developer ID Application certificate signs applications, not packages.
	logs.Reset()
	p12, _ = macostest.Identity(t, macostest.DeveloperIDApplication)
	build(t, signedInfo(t, p12))
	require.Contains(t, logs.String(), "is not a Developer ID Installer certificate")

	logs.Reset()
	p12, _ = macostest.Identity(t, macostest.DeveloperIDInstaller)
	build(t, signedInfo(t, p12))
	require.Empty(t, logs.String())
}
