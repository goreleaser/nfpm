package app_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/app"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/macostest"
	"go.digitalxero.dev/osx/sign"
	"golang.org/x/text/unicode/norm"
	"howett.net/plist"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	identifier = "com.example.foo"
	executable = "/Contents/MacOS/foo"
	// root is the bundle directory at the top of the zip archive.
	root = "foo.app/"
)

// script is a main executable that is not a Mach-O binary. macOS launches
// such a bundle through the interpreter, on any architecture.
const script = "#!/bin/sh\necho foo\n"

// exampleInfo is the smallest application bundle: one executable.
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
					Source:      macostest.WriteFile(t, "foo", script),
					Destination: executable,
					FileInfo:    &files.ContentFileInfo{Mode: 0o755},
				},
			},
			App: nfpm.App{Identifier: identifier},
		},
	})
}

func build(t *testing.T, info *nfpm.Info) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, app.Default.Package(info, &out))
	return out.Bytes()
}

// open builds the bundle and reads the zip archive back.
func open(t *testing.T, info *nfpm.Info) *zip.Reader {
	t.Helper()
	raw := build(t, info)
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	return archive
}

func infoPlist(t *testing.T, archive fs.FS, bundle string) map[string]any {
	t.Helper()
	var values map[string]any
	_, err := plist.Unmarshal(macostest.ReadFile(t, archive, bundle+"Contents/Info.plist"), &values)
	require.NoError(t, err)
	return values
}

// header returns the archive's own record of an entry. The archive's file
// tree view reports every directory as 0555, whatever was stored.
func header(t *testing.T, archive *zip.Reader, name string) *zip.FileHeader {
	t.Helper()
	for _, file := range archive.File {
		if file.Name == name || file.Name == name+"/" {
			return &file.FileHeader
		}
	}
	require.FailNow(t, "the archive has no entry "+name)
	return nil
}

func packageError(t *testing.T, info *nfpm.Info) error {
	t.Helper()
	var out bytes.Buffer
	err := app.Default.Package(info, &out)
	require.Error(t, err)
	return err
}

func TestRegistered(t *testing.T) {
	packager, err := nfpm.Get("app")
	require.NoError(t, err)
	require.Same(t, app.Default, packager)
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".app.zip", app.Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo(t)
	require.Equal(t, "foo_1.2.3_arm64.app.zip", app.Default.ConventionalFileName(info))

	info.Arch = "amd64"
	require.Equal(t, "foo_1.2.3_x86_64.app.zip", app.Default.ConventionalFileName(info))

	info.App.Arch = "universal"
	info.App.Name = "Foo App"
	require.Equal(t, "foo_1.2.3_universal.app.zip", app.Default.ConventionalFileName(info),
		"the file is named after the package, like every other format")
	require.Equal(t, "amd64", info.Arch, "naming a package must not change the info")

	// Packaging reports an unsupported architecture; naming must not hide it.
	info.App.Arch = ""
	info.Arch = "riscv64"
	require.Equal(t, "foo_1.2.3_riscv64.app.zip", app.Default.ConventionalFileName(info))
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
		name := app.Default.ConventionalFileName(info)
		require.NotContains(t, names, name, "%s and %s share a file name", version, names[name])
		names[name] = version
	}
}

func TestBundle(t *testing.T) {
	archive := open(t, exampleInfo(t))

	// The archive holds exactly one bundle directory.
	for _, file := range archive.File {
		require.Contains(t, file.Name, root)
	}

	main := header(t, archive, root+"Contents/MacOS/foo")
	assert.Equal(t, fs.FileMode(0o755), main.Mode(), "the executable bit must survive the archive")
	assert.True(t, macostest.MTime.Equal(main.Modified), main.Modified)
	assert.Equal(t, script, string(macostest.ReadFile(t, archive, root+"Contents/MacOS/foo")))

	assert.Equal(t, "APPL????", string(macostest.ReadFile(t, archive, root+"Contents/PkgInfo")))
	assert.Equal(t, map[string]any{
		"CFBundleExecutable":            "foo",
		"CFBundleIdentifier":            identifier,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleName":                  "foo",
		"CFBundlePackageType":           "APPL",
		"CFBundleShortVersionString":    "1.2.3",
		"CFBundleVersion":               "1.2.3",
	}, infoPlist(t, archive, root))

	_, err := fs.Stat(archive, root+"Contents/_CodeSignature/CodeResources")
	require.ErrorIs(t, err, fs.ErrNotExist, "an unsigned bundle carries no seal")
}

func TestSettings(t *testing.T) {
	info := exampleInfo(t)
	info.App.Name = "Foo App"
	info.App.Executable = "launcher"
	info.App.MinOSVersion = "13.0"
	info.App.InfoPlist = map[string]any{
		"LSUIElement":              true,
		"CFBundleIconFile":         "foo",
		"NSCameraUsageDescription": "Scans codes",
		"CFBundleURLTypes": []any{
			map[string]any{"CFBundleURLSchemes": []any{"foo"}},
		},
	}
	info.Contents[0].Destination = "/Contents/MacOS/launcher"
	archive := open(t, info)

	values := infoPlist(t, archive, "Foo App.app/")
	assert.Equal(t, "Foo App", values["CFBundleName"])
	assert.Equal(t, "launcher", values["CFBundleExecutable"])
	assert.Equal(t, "13.0", values["LSMinimumSystemVersion"])
	assert.Equal(t, true, values["LSUIElement"])
	assert.Equal(t, "foo", values["CFBundleIconFile"])
	assert.Equal(t, "Scans codes", values["NSCameraUsageDescription"])
	assert.Equal(t, []any{map[string]any{"CFBundleURLSchemes": []any{"foo"}}}, values["CFBundleURLTypes"])
	assert.NotEmpty(t, macostest.ReadFile(t, archive, "Foo App.app/Contents/MacOS/launcher"))
}

// The version shown to the user keeps the whole release identity; the build
// version, which macOS compares numerically, is the numeric part.
func TestVersion(t *testing.T) {
	for _, tc := range []struct {
		name          string
		edit          func(*nfpm.Info)
		short, bundle string
	}{
		{"plain", func(*nfpm.Info) {}, "1.2.3", "1.2.3"},
		{"prerelease", func(i *nfpm.Info) { i.Prerelease = "rc1" }, "1.2.3-rc1", "1.2.3"},
		{"metadata", func(i *nfpm.Info) { i.VersionMetadata = "git" }, "1.2.3+git", "1.2.3"},
		{"release", func(i *nfpm.Info) { i.Release = "4" }, "1.2.3-4", "1.2.3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := exampleInfo(t)
			tc.edit(info)
			values := infoPlist(t, open(t, info), root)
			assert.Equal(t, tc.short, values["CFBundleShortVersionString"])
			assert.Equal(t, tc.bundle, values["CFBundleVersion"])
		})
	}
}

// A version macOS cannot read must fail, not be replaced by a made-up one.
func TestVersionMustBeNumeric(t *testing.T) {
	for _, version := range []string{"1.x.3", "latest", "1.2.3.4", "2024-10-01"} {
		info := exampleInfo(t)
		info.VersionSchema = "none"
		info.Version = version
		require.ErrorContains(t, packageError(t, info), `package version "`+version+`" is not valid for a macOS application`)
	}
	for _, version := range []string{"1", "1.2", "2024.10.1"} {
		info := exampleInfo(t)
		info.VersionSchema = "none"
		info.Version = version
		require.Equal(t, version, infoPlist(t, open(t, info), root)["CFBundleVersion"])
	}
}

func TestContents(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      macostest.WriteFile(t, "foo.icns", "icon"),
			Destination: "/Contents/Resources/foo.icns",
			FileInfo:    &files.ContentFileInfo{Mode: 0o644},
		},
		&files.Content{
			Source:      macostest.WriteFile(t, "helper", "#!/bin/sh\n"),
			Destination: "/Contents/MacOS/helper",
			FileInfo:    &files.ContentFileInfo{Mode: 0o750},
		},
		&files.Content{
			Source:      "../MacOS/foo",
			Destination: "/Contents/Resources/launcher",
			Type:        files.TypeSymlink,
		},
		&files.Content{
			Destination: "/Contents/Resources/empty",
			Type:        files.TypeDir,
			FileInfo:    &files.ContentFileInfo{Mode: 0o700},
		},
		&files.Content{
			Source:      macostest.WriteFile(t, "unicode", "héllo\n"),
			Destination: "/Contents/Resources/héllo-世界.txt",
			FileInfo:    &files.ContentFileInfo{Mode: 0o644},
		},
	)
	archive := open(t, info)

	assert.Equal(t, "icon", string(macostest.ReadFile(t, archive, root+"Contents/Resources/foo.icns")))
	assert.Equal(t, fs.FileMode(0o644), header(t, archive, root+"Contents/Resources/foo.icns").Mode())
	assert.Equal(t, fs.FileMode(0o750), header(t, archive, root+"Contents/MacOS/helper").Mode())
	assert.Equal(t, "../MacOS/foo", macostest.ReadLink(t, archive, root+"Contents/Resources/launcher"))
	assert.Equal(t, fs.ModeDir|0o700, header(t, archive, root+"Contents/Resources/empty").Mode())
	// Names are stored decomposed, the way macOS volumes store them.
	assert.Equal(t, "héllo\n",
		string(macostest.ReadFile(t, archive, root+"Contents/Resources/"+norm.NFD.String("héllo-世界.txt"))))
}

func TestTree(t *testing.T) {
	tree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "MacOS"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "Resources", "empty"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "MacOS", "foo"), []byte(script), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "Resources", "data"), []byte("data\n"), 0o644))

	info := exampleInfo(t)
	info.Contents = files.Contents{{
		Source:      tree,
		Destination: "/Contents",
		Type:        files.TypeTree,
		// The mode is given, not taken from disk: a checkout on Windows has
		// no executable bit to take.
		FileInfo: &files.ContentFileInfo{Mode: 0o755},
	}}
	archive := open(t, info)

	assert.Equal(t, script, string(macostest.ReadFile(t, archive, root+"Contents/MacOS/foo")))
	assert.Equal(t, "data\n", string(macostest.ReadFile(t, archive, root+"Contents/Resources/data")))
	assert.True(t, header(t, archive, root+"Contents/Resources/empty").Mode().IsDir(),
		"an empty directory in a tree is kept, not dropped")
}

func TestNonASCIIName(t *testing.T) {
	info := exampleInfo(t)
	info.App.Name = "Café 世界"
	archive := open(t, info)
	require.Equal(t, "Café 世界", infoPlist(t, archive, "Café 世界.app/")["CFBundleName"])
}

func TestContentsForOtherPackagersAreLeftOut(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = append(info.Contents, &files.Content{
		Source:      macostest.WriteFile(t, "linux", "linux\n"),
		Destination: "/usr/bin/foo",
		Packager:    "deb",
	})
	archive := open(t, info)
	for _, file := range archive.File {
		require.NotContains(t, file.Name, "usr")
	}
}

// Destinations are paths inside the bundle. A path of another format must
// fail instead of being put somewhere in the bundle.
func TestDestinationsOutsideTheBundle(t *testing.T) {
	for _, destination := range []string{
		"/usr/local/bin/bar",
		"/MacOS/bar",
		"/Applications/foo.app/Contents/MacOS/bar",
		"/contents/MacOS/bar",
		"/Contents",
	} {
		info := exampleInfo(t)
		info.Contents = append(info.Contents, &files.Content{
			Source:      macostest.WriteFile(t, "bar", "bar"),
			Destination: destination,
		})
		require.ErrorContains(t, packageError(t, info), "start with /Contents/", destination)
	}
}

func TestGeneratedFilesCannotBeSupplied(t *testing.T) {
	for _, destination := range []string{
		"/Contents/Info.plist",
		"/Contents/info.plist",
		"/Contents/PkgInfo",
		"/Contents/CodeResources",
		"/Contents/_CodeSignature/CodeResources",
	} {
		info := exampleInfo(t)
		info.Contents = append(info.Contents, &files.Content{
			Source:      macostest.WriteFile(t, "generated", "x"),
			Destination: destination,
		})
		require.ErrorContains(t, packageError(t, info), "is generated by nfpm", destination)
	}
}

// On Windows, and in a checkout that lost its modes, the main executable is
// not executable on disk. The bundle would not launch, so that is an error
// that says what to set, not a bundle that fails on the user's Mac.
func TestTreeWithoutExecutableMode(t *testing.T) {
	tree := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tree, "MacOS"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tree, "MacOS", "foo"), []byte(script), 0o644))

	info := exampleInfo(t)
	info.Contents = files.Contents{{Source: tree, Destination: "/Contents", Type: files.TypeTree}}
	require.ErrorContains(t, packageError(t, info), "is not executable; set file_info.mode to 0755")
}

func TestMainExecutable(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		info := exampleInfo(t)
		info.Contents[0].Destination = "/Contents/MacOS/other"
		require.ErrorContains(t, packageError(t, info), "must include the main executable at /Contents/MacOS/foo")
	})
	t.Run("named", func(t *testing.T) {
		info := exampleInfo(t)
		info.Contents[0].Destination = "/Contents/MacOS/other"
		info.App.Executable = "other"
		require.Equal(t, "other", infoPlist(t, open(t, info), root)["CFBundleExecutable"])
	})
	t.Run("not executable", func(t *testing.T) {
		info := exampleInfo(t)
		info.Contents[0].FileInfo.Mode = 0o644
		require.ErrorContains(t, packageError(t, info), "is not executable")
	})
	t.Run("symlink", func(t *testing.T) {
		info := exampleInfo(t)
		info.Contents = files.Contents{{Source: "elsewhere", Destination: executable, Type: files.TypeSymlink}}
		require.ErrorContains(t, packageError(t, info), "must be a regular file")
	})
	t.Run("invalid name", func(t *testing.T) {
		info := exampleInfo(t)
		info.App.Executable = "bin/foo"
		require.ErrorContains(t, packageError(t, info), "package app.executable")
	})
}

// A package that says arm64 must not carry an Intel program: it would not
// launch on the Macs it claims to support.
func TestExecutableArchitecture(t *testing.T) {
	arm64 := macostest.Program(t, "arm64")
	amd64 := macostest.Program(t, "amd64")
	universal := macostest.UniversalProgram(t)

	for _, tc := range []struct {
		name, arch, program, wantError string
	}{
		{"arm64 program", "arm64", arm64, ""},
		{"amd64 program", "amd64", amd64, ""},
		{"universal program", "all", universal, ""},
		{"universal program in a single-architecture package", "arm64", universal, ""},
		{"script", "all", "", ""},
		{"amd64 program in an arm64 package", "arm64", amd64, "is not built for arm64"},
		{"arm64 program in an amd64 package", "amd64", arm64, "is not built for x86_64"},
		{"arm64 program in a universal package", "all", arm64, "is not built for x86_64"},
		{"amd64 program in a universal package", "universal", amd64, "is not built for arm64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := exampleInfo(t)
			info.Arch = tc.arch
			if tc.program != "" {
				info.Contents[0].Source = tc.program
			}
			if tc.wantError != "" {
				require.ErrorContains(t, packageError(t, info), tc.wantError)
				return
			}
			want, err := os.ReadFile(info.Contents[0].Source)
			require.NoError(t, err)
			require.Equal(t, want, macostest.ReadFile(t, open(t, info), root+"Contents/MacOS/foo"),
				"an unsigned bundle carries the program unchanged")
		})
	}
}

// A Mach-O binary for a processor no current Mac has is not built for either
// supported architecture.
func TestExecutableForAnotherProcessor(t *testing.T) {
	// The 32-byte header of an empty 64-bit Mach-O executable for PowerPC.
	header := make([]byte, 32)
	binary.BigEndian.PutUint32(header[0:], 0xfeedfacf) // magic, big-endian 64-bit
	binary.BigEndian.PutUint32(header[4:], 0x01000012) // cputype: PowerPC 64
	binary.BigEndian.PutUint32(header[12:], 2)         // filetype: executable
	path := filepath.Join(t.TempDir(), "ppc")
	require.NoError(t, os.WriteFile(path, header, 0o755))

	info := exampleInfo(t)
	info.Contents[0].Source = path
	require.ErrorContains(t, packageError(t, info), "is not built for arm64")
}

func TestUnsupportedArch(t *testing.T) {
	for _, arch := range []string{"386", "arm", "riscv64"} {
		info := exampleInfo(t)
		info.Arch = arch
		require.ErrorContains(t, packageError(t, info), `package arch "`+arch+`" is not supported by macOS`)
	}
	info := exampleInfo(t)
	info.App.Arch = "amd64"
	require.ErrorContains(t, packageError(t, info), `package app.arch "amd64" is not supported`)
}

func TestIdentifier(t *testing.T) {
	info := exampleInfo(t)
	info.App.Identifier = ""
	require.ErrorContains(t, packageError(t, info), "package app.identifier must be provided")

	for _, invalid := range []string{"com.example.foo bar", ".com.example", "com.example.foo_bar", "cömpany.foo"} {
		info := exampleInfo(t)
		info.App.Identifier = invalid
		require.ErrorContains(t, packageError(t, info), "package app.identifier", invalid)
	}
}

func TestMinOSVersion(t *testing.T) {
	for _, invalid := range []string{"13.x", "v13", "13.0.0.1"} {
		info := exampleInfo(t)
		info.App.MinOSVersion = invalid
		require.ErrorContains(t, packageError(t, info), "package app.min_os_version", invalid)
	}
}

func TestInvalidName(t *testing.T) {
	for _, name := range []string{"Foo/Bar", "Foo:Bar", ".", ".."} {
		info := exampleInfo(t)
		info.App.Name = name
		require.ErrorContains(t, packageError(t, info), "package app.name", name)
	}
}

// The keys nfpm writes from other settings cannot also be set by hand: the
// property list would have two sources for one value.
func TestReservedInfoPlistKeys(t *testing.T) {
	for key, setting := range map[string]string{
		"CFBundleExecutable":            "use app.executable",
		"CFBundleIdentifier":            "use app.identifier",
		"CFBundleName":                  "use app.name",
		"CFBundleShortVersionString":    "use version",
		"CFBundleVersion":               "use version",
		"LSMinimumSystemVersion":        "use app.min_os_version",
		"CFBundlePackageType":           "cannot be changed",
		"CFBundleInfoDictionaryVersion": "cannot be changed",
	} {
		info := exampleInfo(t)
		info.App.InfoPlist = map[string]any{key: "x"}
		err := packageError(t, info)
		require.ErrorContains(t, err, `app.info_plist["`+key+`"]`)
		require.ErrorContains(t, err, setting)
	}
}

func TestInfoPlistValuesMustBeEncodable(t *testing.T) {
	info := exampleInfo(t)
	info.App.InfoPlist = map[string]any{"Outer": []any{map[string]any{"Inner": nil}}}
	require.ErrorContains(t, packageError(t, info), `app.info_plist["Outer"][0]["Inner"]`)
}

// macOS volumes ignore letter case and Unicode normalization, so these are
// one path there.
func TestCollidingDestinations(t *testing.T) {
	for name, pair := range map[string][2]string{
		"file case":      {"/Contents/Resources/a", "/Contents/Resources/A"},
		"directory case": {"/Contents/Resources/x", "/Contents/resources/y"},
		"normalization":  {"/Contents/Resources/café", "/Contents/Resources/café"},
	} {
		t.Run(name, func(t *testing.T) {
			info := exampleInfo(t)
			info.Contents = append(info.Contents,
				&files.Content{Source: macostest.WriteFile(t, "a", "a"), Destination: pair[0]},
				&files.Content{Source: macostest.WriteFile(t, "b", "b"), Destination: pair[1]},
			)
			require.ErrorContains(t, packageError(t, info), "macOS stores them as the same path")
		})
	}
}

func TestUnsupportedSettingsWarn(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	info := exampleInfo(t)
	info.Depends = []string{"bash"}
	info.Provides = []string{"qux"}
	info.Scripts = nfpm.Scripts{
		PreInstall:  macostest.WriteFile(t, "preinstall", "#!/bin/sh\necho SCRIPT_BODY\n"),
		PostInstall: macostest.WriteFile(t, "postinstall", "#!/bin/sh\necho SCRIPT_BODY\n"),
		PreRemove:   macostest.WriteFile(t, "preremove", "#!/bin/sh\necho SCRIPT_BODY\n"),
		PostRemove:  macostest.WriteFile(t, "postremove", "#!/bin/sh\necho SCRIPT_BODY\n"),
	}
	info.Contents[0].FileInfo.Owner = "alice"
	raw := build(t, info)

	for _, want := range []string{
		"depends is ignored", "provides is ignored",
		"the preinstall script is ignored", "the postinstall script is ignored",
		"the preremove script is ignored", "the postremove script is ignored",
		"the owner and group of /Contents/MacOS/foo are ignored",
	} {
		assert.Contains(t, logs.String(), "warning: app: ")
		assert.Contains(t, logs.String(), want)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	for _, file := range archive.File {
		if file.Mode().IsRegular() {
			require.NotContains(t, string(macostest.ReadFile(t, archive, file.Name)), "SCRIPT_BODY")
		}
	}
}

func TestNoWarningsForAPlainBundle(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	build(t, exampleInfo(t))
	require.Empty(t, logs.String())
}

func TestReproducible(t *testing.T) {
	info := exampleInfo(t)
	first := build(t, info)
	require.Equal(t, first, build(t, info), "the same input must give the same bytes")

	require.NoError(t, os.WriteFile(info.Contents[0].Source, []byte("#!/bin/sh\necho changed\n"), 0o644))
	require.NotEqual(t, first, build(t, info), "changed content must change the bundle")
}

func TestPackageDoesNotChangeTheInfo(t *testing.T) {
	info := exampleInfo(t)
	info.Arch = "amd64"
	contents := info.Contents
	build(t, info)
	require.Equal(t, "amd64", info.Arch)
	require.Equal(t, contents, info.Contents)
	require.Len(t, info.Contents, 1)
}

func signedInfo(t *testing.T, p12 string) *nfpm.Info {
	t.Helper()
	info := exampleInfo(t)
	info.Contents[0].Source = macostest.Program(t, "arm64")
	info.App.Signature = nfpm.AppSignature{
		P12File:       p12,
		TimestampURL:  "none",
		KeyPassphrase: macostest.Passphrase,
	}
	return info
}

func TestSigning(t *testing.T) {
	p12, certificate := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      macostest.Program(t, "arm64"),
			Destination: "/Contents/MacOS/helper",
			FileInfo:    &files.ContentFileInfo{Mode: 0o755},
		},
		&files.Content{
			Source:      macostest.WriteFile(t, "data", "data\n"),
			Destination: "/Contents/Resources/data",
		},
	)
	archive := open(t, info)

	// Every program in the bundle is signed with the identity, not only the
	// main one: macOS rejects a bundle with unsigned code inside.
	for _, program := range []string{"Contents/MacOS/foo", "Contents/MacOS/helper"} {
		signature := macostest.CodeSignature(t, macostest.ReadFile(t, archive, root+program))
		require.NotEmpty(t, signature, program)
		require.True(t, bytes.Contains(signature, certificate.Raw),
			"%s is not signed with the configured identity", program)
	}

	// The seal covers the bundle's other files.
	seal := string(macostest.ReadFile(t, archive, root+"Contents/_CodeSignature/CodeResources"))
	require.Contains(t, seal, "Resources/data")
	require.Contains(t, seal, "MacOS/helper")
}

// The program built by Go carries the linker's ad hoc signature. Without a
// configured identity nfpm leaves it exactly as it is.
func TestUnsignedBundleKeepsTheProgramUntouched(t *testing.T) {
	_, certificate := macostest.Identity(t)
	info := exampleInfo(t)
	info.Contents[0].Source = macostest.Program(t, "arm64")
	archive := open(t, info)

	want, err := os.ReadFile(info.Contents[0].Source)
	require.NoError(t, err)
	program := macostest.ReadFile(t, archive, root+"Contents/MacOS/foo")
	require.Equal(t, want, program)
	require.False(t, bytes.Contains(macostest.CodeSignature(t, program), certificate.Raw))
}

func TestEntitlements(t *testing.T) {
	const entitlement = "com.apple.security.cs.allow-jit"
	entitlements := macostest.WriteFile(t, "app.entitlements", `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>`+entitlement+`</key><true/></dict></plist>
`)
	p12, _ := macostest.Identity(t)

	info := signedInfo(t, p12)
	info.App.Signature.Entitlements = entitlements
	signature := macostest.CodeSignature(t, macostest.ReadFile(t, open(t, info), root+"Contents/MacOS/foo"))
	require.True(t, bytes.Contains(signature, []byte(entitlement)))

	plain := macostest.CodeSignature(t, macostest.ReadFile(t, open(t, signedInfo(t, p12)), root+"Contents/MacOS/foo"))
	require.False(t, bytes.Contains(plain, []byte(entitlement)))

	t.Run("missing file", func(t *testing.T) {
		info := signedInfo(t, p12)
		info.App.Signature.Entitlements = filepath.Join(t.TempDir(), "missing")
		require.ErrorContains(t, packageError(t, info), "reading entitlements")
	})
	t.Run("without an identity", func(t *testing.T) {
		info := exampleInfo(t)
		info.App.Signature.Entitlements = entitlements
		require.ErrorContains(t, packageError(t, info), "app.signature.entitlements needs app.signature.p12_file")
	})
}

// nfpm cannot sign a bundle inside the bundle. Saying so is the only safe
// answer: a seal over unsigned nested code is one macOS rejects.
func TestSigningRejectsNestedBundles(t *testing.T) {
	nested := &files.Content{
		Source:      macostest.Program(t, "arm64"),
		Destination: "/Contents/Frameworks/Bar.framework/Versions/A/Bar",
		FileInfo:    &files.ContentFileInfo{Mode: 0o755},
	}

	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.Contents = append(info.Contents, nested)
	err := packageError(t, info)
	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &signing)
	require.ErrorIs(t, err, sign.ErrNestedBundle)

	// Unsigned, the same bundle is packaged as it is.
	info = exampleInfo(t)
	info.Contents = append(info.Contents, nested)
	archive := open(t, info)
	require.NotEmpty(t, macostest.ReadFile(t, archive, root+"Contents/Frameworks/Bar.framework/Versions/A/Bar"))
}

func TestSigningWrongPassphrase(t *testing.T) {
	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.App.Signature.KeyPassphrase = "wrong"
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
	info.App.Signature.TimestampURL = server.URL
	err := packageError(t, info)

	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &signing)
	require.ErrorIs(t, err, sign.ErrTimestamp)
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// The default is Apple's timestamp service, as codesign does. The request is
// intercepted, so the test proves where it would go without going there.
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
	info.App.Signature.TimestampURL = ""
	require.ErrorIs(t, packageError(t, info), sign.ErrTimestamp)
	require.Contains(t, requested, "timestamp.apple.com")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// A bundle that cannot be written is not a signing problem.
func TestWriteFailureIsNotASigningFailure(t *testing.T) {
	p12, _ := macostest.Identity(t)
	err := app.Default.Package(signedInfo(t, p12), failingWriter{})
	require.ErrorContains(t, err, "disk full")
	var signing *nfpm.ErrSigningFailure
	require.NotErrorAs(t, err, &signing)
}

func TestCertificateKindWarning(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	p12, _ := macostest.Identity(t)
	build(t, signedInfo(t, p12))
	require.Contains(t, logs.String(), "is not a Developer ID Application certificate")

	// A Developer ID Installer certificate signs packages, not applications.
	logs.Reset()
	p12, _ = macostest.Identity(t, macostest.DeveloperIDInstaller)
	build(t, signedInfo(t, p12))
	require.Contains(t, logs.String(), "is not a Developer ID Application certificate")

	logs.Reset()
	p12, _ = macostest.Identity(t, macostest.DeveloperIDApplication)
	build(t, signedInfo(t, p12))
	require.Empty(t, logs.String())
}
