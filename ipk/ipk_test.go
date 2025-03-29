package ipk

import (
	"archive/tar"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nolint: gochecknoglobals
var update = flag.Bool("update", false, "update .golden files")

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "v1.0.0",
		Section:     "default",
		Homepage:    "http://carlosbecker.com",
		Vendor:      "nope",
		Overridables: nfpm.Overridables{
			Depends: []string{
				"bash",
			},
			Recommends: []string{
				"git",
			},
			Suggests: []string{
				"bash",
			},
			Replaces: []string{
				"svn",
			},
			Provides: []string{
				"bzr",
			},
			Conflicts: []string{
				"zsh",
			},
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/usr/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/usr/share/doc/fake/fake.txt",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        files.TypeConfig,
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake2.conf",
					Type:        files.TypeConfigNoReplace,
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake3.conf",
					Type:        files.TypeConfigNoReplace,
				},
				{
					Destination: "/var/log/whatever",
					Type:        files.TypeDir,
				},
				{
					Destination: "/usr/share/whatever",
					Type:        files.TypeDir,
				},
			},
			IPK: nfpm.IPK{
				Predepends: []string{"less"},
			},
		},
	})
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".ipk", Default.ConventionalExtension())
}

func TestIPK(t *testing.T) {
	for _, arch := range []string{"386", "amd64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch
			err := Default.Package(info, io.Discard)
			require.NoError(t, err)
		})
	}
}

func TestIPKPlatform(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.deb")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	info := exampleInfo()
	info.Platform = "darwin"
	err = Default.Package(info, f)
	require.NoError(t, err)
}

func extractIPKArchitecture(deb *bytes.Buffer) string {
	for _, s := range strings.Split(deb.String(), "\n") {
		if strings.Contains(s, "Architecture: ") {
			return strings.TrimPrefix(s, "Architecture: ")
		}
	}
	return ""
}

func splitIPKArchitecture(deb *bytes.Buffer) (string, string) {
	a := extractIPKArchitecture(deb)
	if strings.Contains(a, "-") {
		f := strings.Split(a, "-")
		return f[0], f[1]
	}
	return "linux", a
}

func TestIPKOS(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	o, _ := splitIPKArchitecture(&buf)
	require.Equal(t, "linux", o)
}

func TestIPKArch(t *testing.T) {
	info := exampleInfo()
	var buf bytes.Buffer
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	_, a := splitIPKArchitecture(&buf)
	require.Equal(t, "amd64", a)
}

func extractIPKVersion(deb *bytes.Buffer) string {
	for _, s := range strings.Split(deb.String(), "\n") {
		if strings.Contains(s, "Version: ") {
			return strings.TrimPrefix(s, "Version: ")
		}
	}
	return ""
}

func TestIPKVersionWithDash(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0-beta"
	err := Default.Package(info, io.Discard)
	require.NoError(t, err)
}

func TestIPKVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	var buf bytes.Buffer
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractIPKVersion(&buf)
	require.Equal(t, "1.0.0", v)
}

func TestIPKVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "1"
	var buf bytes.Buffer
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractIPKVersion(&buf)
	require.Equal(t, "1.0.0-1", v)
}

func TestIPKVersionWithPrerelease(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Prerelease = "1"
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractIPKVersion(&buf)
	require.Equal(t, "1.0.0~1", v)
}

func TestIPKVersionWithReleaseAndPrerelease(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "2"
	info.Prerelease = "rc1" //nolint:golint,goconst
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractIPKVersion(&buf)
	require.Equal(t, "1.0.0~rc1-2", v)
}

func TestIPKVersionWithVersionMetadata(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0+meta" //nolint:golint,goconst
	info.VersionMetadata = ""
	err := renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractIPKVersion(&buf)
	require.Equal(t, "1.0.0+meta", v)

	buf.Reset()

	info.Version = "1.0.0" //nolint:golint,goconst
	info.VersionMetadata = "meta"
	err = renderControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v = extractIPKVersion(&buf)
	require.Equal(t, "1.0.0+meta", v)

	buf.Reset()

	info.Version = "1.0.0+foo" //nolint:golint,goconst
	info.Prerelease = "alpha"
	info.VersionMetadata = "meta"
	err = renderControl(&buf, controlData{nfpm.WithDefaults(info), 0})
	require.NoError(t, err)
	v = extractIPKVersion(&buf)
	require.Equal(t, "1.0.0~alpha+meta", v)
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info:          exampleInfo(),
		InstalledSize: 10,
	}))
	golden := "testdata/control.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestNoJoinsControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "v1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends:    []string{},
				Recommends: []string{},
				Suggests:   []string{},
				Replaces:   []string{},
				Provides:   []string{},
				Conflicts:  []string{},
				Contents:   []*files.Content{},
			},
		}),
		InstalledSize: 10,
	}))
	golden := "testdata/control2.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestVersionControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "v1.0.0-beta+meta",
			Release:     "2",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends:    []string{},
				Recommends: []string{},
				Suggests:   []string{},
				Replaces:   []string{},
				Provides:   []string{},
				Conflicts:  []string{},
				Contents:   []*files.Content{},
			},
		}),
		InstalledSize: 10,
	}))
	golden := "testdata/control4.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestIPKFileDoesNotExist(t *testing.T) {
	abs, err := filepath.Abs("../testdata/whatever.confzzz")
	require.NoError(t, err)
	err = Default.Package(
		nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends: []string{
					"bash",
				},
				Contents: []*files.Content{
					{
						Source:      "../testdata/fake",
						Destination: "/usr/bin/fake",
					},
					{
						Source:      "../testdata/whatever.confzzz",
						Destination: "/etc/fake/fake.conf",
						Type:        files.TypeConfig,
					},
				},
			},
		}),
		io.Discard,
	)
	require.EqualError(t, err, fmt.Sprintf("matching \"%s\": file does not exist", filepath.ToSlash(abs)))
}

func TestIPKNoFiles(t *testing.T) {
	err := Default.Package(
		nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Overridables: nfpm.Overridables{
				Depends: []string{
					"bash",
				},
			},
		}),
		io.Discard,
	)
	require.NoError(t, err)
}

func TestIPKNoInfo(t *testing.T) {
	err := Default.Package(nfpm.WithDefaults(&nfpm.Info{}), io.Discard)
	require.Error(t, err)
}

func TestConffiles(t *testing.T) {
	info := nfpm.WithDefaults(&nfpm.Info{
		Name:        "minimal",
		Arch:        "arm64",
		Description: "Minimal does nothing",
		Priority:    "extra",
		Version:     "1.0.0",
		Section:     "default",
		Maintainer:  "maintainer",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/etc/fake",
					Type:        files.TypeConfig,
				},
			},
		},
	})
	err := nfpm.PrepareForPackager(info, packagerName)
	require.NoError(t, err)
	out := conffiles(info)
	require.Equal(t, "/etc/fake\n", string(out), "should have a trailing empty line")
}

func TestMinimalFields(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "minimal",
			Arch:        "arm64",
			Description: "Minimal does nothing",
			Priority:    "extra",
			Version:     "1.0.0",
			Maintainer:  "maintainer",
		}),
	}))
	golden := "testdata/minimal.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestIPKEpoch(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "withepoch",
			Arch:        "arm64",
			Description: "Has an epoch added to it's version",
			Priority:    "extra",
			Epoch:       "2",
			Version:     "1.0.0",
			Section:     "default",
		}),
	}))
	golden := "testdata/withepoch.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestMultilineFields(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "multiline",
			Arch:        "riscv64",
			Description: "This field is a\nmultiline field\n\nthat should work.",
			Priority:    "extra",
			Version:     "1.0.0",
			Maintainer:  "someone",
		}),
	}))
	golden := "testdata/multiline.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestIPKConventionalFileName(t *testing.T) {
	info := &nfpm.Info{
		Name:       "testpkg",
		Arch:       "all",
		Maintainer: "maintainer",
	}

	testCases := []struct {
		Version    string
		Release    string
		Prerelease string
		Expected   string
		Metadata   string
	}{
		{
			Version: "1.2.3", Release: "", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3_%s.ipk", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "4", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3-4_%s.ipk", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "4", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3~5-4_%s.ipk", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3~5_%s.ipk", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "1", Prerelease: "5", Metadata: "git",
			Expected: fmt.Sprintf("%s_1.2.3~5+git-1_%s.ipk", info.Name, info.Arch),
		},
	}

	for _, testCase := range testCases {
		info.Version = testCase.Version
		info.Release = testCase.Release
		info.Prerelease = testCase.Prerelease
		info.VersionMetadata = testCase.Metadata

		require.Equal(t, testCase.Expected, Default.ConventionalFileName(info))
	}
}

func TestSymlink(t *testing.T) {
	var (
		configFilePath = "/usr/share/doc/fake/fake.txt"
		symlink        = "/path/to/symlink"
		symlinkTarget  = configFilePath
	)

	info := &nfpm.Info{
		Name:        "symlink-in-files",
		Arch:        "amd64",
		Description: "This package's config references a file via symlink.",
		Version:     "1.0.0",
		Maintainer:  "maintainer",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/whatever.conf",
					Destination: configFilePath,
				},
				{
					Source:      symlinkTarget,
					Destination: symlink,
					Type:        files.TypeSymlink,
				},
			},
		},
	}
	err := nfpm.PrepareForPackager(info, packagerName)
	require.NoError(t, err)

	var buf bytes.Buffer
	tarball := tar.NewWriter(&buf)
	_, err = populateDataTar(info, tarball)
	require.NoError(t, err)
	require.NoError(t, tarball.Close())

	packagedSymlinkHeader := extractFileHeaderFromTar(t, buf.Bytes(), symlink)

	require.Equal(t, symlink, path.Join("/", packagedSymlinkHeader.Name)) // nolint:gosec
	require.Equal(t, uint8(tar.TypeSymlink), packagedSymlinkHeader.Typeflag)
	require.Equal(t, symlinkTarget, packagedSymlinkHeader.Linkname)
}

func TestEnsureRelativePrefixInTarballs(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "/symlink/to/fake.txt",
			Destination: "/usr/share/doc/fake/fake.txt",
			Type:        files.TypeSymlink,
		},
	}
	info.Changelog = "../testdata/changelog.yaml"
	err := nfpm.PrepareForPackager(info, packagerName)
	require.NoError(t, err)

	var dataBuf bytes.Buffer
	dataTarball := tar.NewWriter(&dataBuf)
	instSize, err := populateDataTar(info, dataTarball)
	require.NoError(t, err)
	require.NoError(t, dataTarball.Close())
	testRelativePathPrefixInTar(t, dataBuf.Bytes())

	var controlBuf bytes.Buffer
	controlTarball := tar.NewWriter(&controlBuf)
	err = populateControlTar(info, controlTarball, instSize)
	require.NoError(t, err)
	require.NoError(t, controlTarball.Close())
	testRelativePathPrefixInTar(t, controlBuf.Bytes())
}

func TestDirectories(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/foo/file",
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/bar/file",
		},
		{
			Destination: "/etc/bar",
			Type:        files.TypeDir,
			FileInfo: &files.ContentFileInfo{
				Owner: "test",
				Mode:  0o700,
			},
		},
		{
			Destination: "/etc/baz",
			Type:        files.TypeDir,
		},
		{
			Destination: "/usr/lib/something/somethingelse",
			Type:        files.TypeDir,
		},
	}

	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	var dataBuf bytes.Buffer
	tarball := tar.NewWriter(&dataBuf)
	_, err := populateDataTar(info, tarball)
	require.NoError(t, err)
	require.NoError(t, tarball.Close())

	dataTarball := dataBuf.Bytes()

	require.Equal(t, []string{
		"./etc/",
		"./etc/bar/",
		"./etc/bar/file",
		"./etc/baz/",
		"./etc/foo/",
		"./etc/foo/file",
		"./usr/",
		"./usr/lib/",
		"./usr/lib/something/",
		"./usr/lib/something/somethingelse/",
	}, getTree(t, dataBuf.Bytes()))

	// for ipk all implicit or explicit directories are created in the tarball
	h := extractFileHeaderFromTar(t, dataTarball, "/etc")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/etc/foo")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/etc/bar")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	require.Equal(t, int64(0o700), h.Mode)
	require.Equal(t, "test", h.Uname)
	h = extractFileHeaderFromTar(t, dataTarball, "/etc/baz")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))

	h = extractFileHeaderFromTar(t, dataTarball, "/usr")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/usr/lib")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/usr/lib/something")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/usr/lib/something/somethingelse")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
}

func TestNoDuplicateContents(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/foo/file",
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/foo/file2",
		},
		{
			Destination: "/etc/foo",
			Type:        files.TypeDir,
		},
		{
			Destination: "/etc/baz",
			Type:        files.TypeDir,
		},
	}

	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	var dataBuf bytes.Buffer
	tarball := tar.NewWriter(&dataBuf)
	_, err := populateDataTar(info, tarball)
	require.NoError(t, err)
	require.NoError(t, tarball.Close())

	exists := map[string]bool{}

	tr := tar.NewReader(bytes.NewReader(dataBuf.Bytes()))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(t, err)

		_, ok := exists[hdr.Name]
		if ok {
			t.Fatalf("%s exists more than once in tarball", hdr.Name)
		}

		exists[hdr.Name] = true
	}
}

func testRelativePathPrefixInTar(tb testing.TB, tarFile []byte) {
	tb.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)
		require.True(tb, strings.HasPrefix(hdr.Name, "./"), "%s does not start with './'", hdr.Name)
	}
}

func TestDisableGlobbing(t *testing.T) {
	info := exampleInfo()
	info.DisableGlobbing = true
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/{file}[",
			Destination: "/test/{file}[",
		},
	}
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	var dataBuf bytes.Buffer
	tarball := tar.NewWriter(&dataBuf)
	_, err := populateDataTar(info, tarball)
	require.NoError(t, err)
	require.NoError(t, tarball.Close())

	expectedContent, err := os.ReadFile("../testdata/{file}[")
	require.NoError(t, err)

	actualContent := extractFileFromTar(t, dataBuf.Bytes(), "/test/{file}[")

	require.Equal(t, expectedContent, actualContent)
}

func TestNoDuplicateAutocreatedDirectories(t *testing.T) {
	info := exampleInfo()
	info.DisableGlobbing = true
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/fake",
			Destination: "/etc/foo/bar",
		},
		{
			Type:        files.TypeDir,
			Destination: "/etc/foo",
		},
	}
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	expected := map[string]bool{
		"./etc/":        true,
		"./etc/foo/":    true,
		"./etc/foo/bar": true,
	}

	var dataBuf bytes.Buffer
	tarball := tar.NewWriter(&dataBuf)
	_, err := populateDataTar(info, tarball)
	require.NoError(t, err)
	require.NoError(t, tarball.Close())

	contents := tarContents(t, dataBuf.Bytes())

	if len(expected) != len(contents) {
		t.Fatalf("contents has %d entries instead of %d: %#v", len(contents), len(expected), contents)
	}

	for _, entry := range contents {
		if !expected[entry] {
			t.Fatalf("unexpected content: %q", entry)
		}
	}
}

func TestNoDuplicateDirectories(t *testing.T) {
	info := exampleInfo()
	info.DisableGlobbing = true
	info.Contents = []*files.Content{
		{
			Type:        files.TypeDir,
			Destination: "/etc/foo",
		},
		{
			Type:        files.TypeDir,
			Destination: "/etc/foo/",
		},
	}
	require.Error(t, nfpm.PrepareForPackager(info, packagerName))
}

func TestIgnoreUnrelatedFiles(t *testing.T) {
	info := exampleInfo()
	info.Contents = files.Contents{
		{
			Source:      "../testdata/fake",
			Destination: "/usr/bin/fake",
			Packager:    "rpm",
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/usr/share/doc/fake/fake.txt",
			Type:        files.TypeRPMLicence,
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/fake/fake.conf",
			Type:        files.TypeRPMLicense,
		},
		{
			Source:      "../testdata/whatever.conf",
			Destination: "/etc/fake/fake2.conf",
			Type:        files.TypeRPMReadme,
		},
		{
			Destination: "/var/log/whatever",
			Type:        files.TypeRPMDoc,
		},
	}

	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))

	var dataBuf bytes.Buffer
	tarball := tar.NewWriter(&dataBuf)
	_, err := populateDataTar(info, tarball)
	require.NoError(t, err)
	require.NoError(t, tarball.Close())

	contents := tarContents(t, dataBuf.Bytes())
	require.Empty(t, contents)
}

func TestEmptyButRequiredIPKFields(t *testing.T) {
	item := nfpm.WithDefaults(&nfpm.Info{
		Name:    "foo",
		Version: "v1.0.0",
	})
	Default.SetPackagerDefaults(item)

	require.Equal(t, "optional", item.Priority)
	require.Equal(t, "Unset Maintainer <unset@localhost>", item.Maintainer)

	var deb bytes.Buffer
	err := Default.Package(item, &deb)
	require.NoError(t, err)
}

func TestArches(t *testing.T) {
	for k := range archToIPK {
		t.Run(k, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = k
			info = ensureValidArch(info)
			require.Equal(t, archToIPK[k], info.Arch)
		})
	}

	t.Run("override", func(t *testing.T) {
		info := exampleInfo()
		info.IPK.Arch = "foo64"
		info = ensureValidArch(info)
		require.Equal(t, "foo64", info.Arch)
	})
}

func TestFields(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "v1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Overridables: nfpm.Overridables{
				IPK: nfpm.IPK{
					Fields: map[string]string{
						"Bugs":  "https://github.com/goreleaser/nfpm/issues",
						"Empty": "",
					},
				},
			},
		}),
		InstalledSize: 10,
	}))
	golden := "testdata/control3.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestMost(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "v1.0.0",
			Section:     "default",
			License:     "MIT",
			Homepage:    "http://carlosbecker.com",
			Overridables: nfpm.Overridables{
				IPK: nfpm.IPK{
					ABIVersion:    "1",
					AutoInstalled: true,
					Essential:     true,
					Fields: map[string]string{
						"Bugs":  "https://github.com/goreleaser/nfpm/issues",
						"Empty": "",
					},
				},
			},
		}),
		InstalledSize: 10,
	}))
	golden := "testdata/control_most.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestGlob(t *testing.T) {
	require.NoError(t, Default.Package(nfpm.WithDefaults(&nfpm.Info{
		Name:       "nfpm-repro",
		Version:    "1.0.0",
		Maintainer: "asdfasdf",

		Overridables: nfpm.Overridables{
			Contents: files.Contents{
				{
					Destination: "/usr/share/nfpm-repro",
					Source:      "../files/*",
				},
			},
		},
	}), io.Discard))
}

func TestBadProvides(t *testing.T) {
	var w bytes.Buffer
	info := exampleInfo()
	info.Provides = []string{"  "}
	require.NoError(t, renderControl(&w, controlData{
		Info: nfpm.WithDefaults(info),
	}))
	golden := "testdata/bad_provides.golden"
	if *update {
		require.NoError(t, os.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := os.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func Test_stripDisallowedFields(t *testing.T) {
	tests := []struct {
		description string
		info        *nfpm.Info
		expect      map[string]string
	}{
		{
			description: "",
			info: &nfpm.Info{
				Overridables: nfpm.Overridables{
					IPK: nfpm.IPK{
						ABIVersion:    "1",
						AutoInstalled: true,
						Essential:     true,
						Fields: map[string]string{
							"Bugs":           "https://github.com/goreleaser/nfpm/issues",
							"Empty":          "",
							"Conffiles":      "removed",
							"Filename":       "removed",
							"Installed-Time": "removed",
							"MD5sum":         "removed",
							"SHA256sum":      "removed",
							"Size":           "removed",
							"size":           "removed",
							"Status":         "removed",
							"Source":         "ok",
						},
					},
				},
			},
			expect: map[string]string{
				"Bugs":   "https://github.com/goreleaser/nfpm/issues",
				"Empty":  "",
				"Source": "ok",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert := assert.New(t)

			stripDisallowedFields(tc.info)

			assert.Equal(tc.expect, tc.info.IPK.Fields)
		})
	}
}
