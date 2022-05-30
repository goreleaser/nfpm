package deb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5" // nolint: gosec
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/blakesmith/ar"
	"github.com/goreleaser/chglog"
	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
	"github.com/stretchr/testify/require"
	"github.com/xi2/xz"
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
					Destination: "/usr/local/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/usr/share/doc/fake/fake.txt",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        "config",
				},
				{
					Destination: "/var/log/whatever",
					Type:        "dir",
				},
				{
					Destination: "/usr/share/whatever",
					Type:        "dir",
				},
			},
		},
	})
}

func TestDeb(t *testing.T) {
	for _, arch := range []string{"386", "amd64"} {
		arch := arch
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch
			err := Default.Package(info, ioutil.Discard)
			require.NoError(t, err)
		})
	}
}

func extractDebVersion(deb *bytes.Buffer) string {
	for _, s := range strings.Split(deb.String(), "\n") {
		if strings.Contains(s, "Version: ") {
			return strings.TrimPrefix(s, "Version: ")
		}
	}
	return ""
}

func TestDebVersionWithDash(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0-beta"
	err := Default.Package(info, ioutil.Discard)
	require.NoError(t, err)
}

func TestDebVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	var buf bytes.Buffer
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	require.Equal(t, "1.0.0", v)
}

func TestDebVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "1"
	var buf bytes.Buffer
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	require.Equal(t, "1.0.0-1", v)
}

func TestDebVersionWithPrerelease(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Prerelease = "1"
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	require.Equal(t, "1.0.0~1", v)
}

func TestDebVersionWithReleaseAndPrerelease(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "2"
	info.Prerelease = "rc1" //nolint:golint,goconst
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	require.Equal(t, "1.0.0~rc1-2", v)
}

func TestDebVersionWithVersionMetadata(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0+meta" //nolint:golint,goconst
	info.VersionMetadata = ""
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	require.Equal(t, "1.0.0+meta", v)

	buf.Reset()

	info.Version = "1.0.0" //nolint:golint,goconst
	info.VersionMetadata = "meta"
	err = writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v = extractDebVersion(&buf)
	require.Equal(t, "1.0.0+meta", v)

	buf.Reset()

	info.Version = "1.0.0+foo" //nolint:golint,goconst
	info.Prerelease = "alpha"
	info.VersionMetadata = "meta"
	err = writeControl(&buf, controlData{nfpm.WithDefaults(info), 0})
	require.NoError(t, err)
	v = extractDebVersion(&buf)
	require.Equal(t, "1.0.0~alpha+meta", v)
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(),
		InstalledSize: 10,
	}))
	golden := "testdata/control.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestSpecialFiles(t *testing.T) {
	var w bytes.Buffer
	out := tar.NewWriter(&w)
	filePath := "testdata/templates.golden"
	require.Error(t, newFilePathInsideTar(out, "doesnotexit", "templates", 0o644))
	require.NoError(t, newFilePathInsideTar(out, filePath, "templates", 0o644))
	in := tar.NewReader(&w)
	header, err := in.Next()
	require.NoError(t, err)
	require.Equal(t, "templates", header.FileInfo().Name())
	mode, err := strconv.ParseInt("0644", 8, 64)
	require.NoError(t, err)
	require.Equal(t, int64(header.FileInfo().Mode()), mode)
	data, err := ioutil.ReadAll(in)
	require.NoError(t, err)
	org, err := ioutil.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, data, org)
}

func TestNoJoinsControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
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
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestVersionControl(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
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
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestDebFileDoesNotExist(t *testing.T) {
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
						Destination: "/usr/local/bin/fake",
					},
					{
						Source:      "../testdata/whatever.confzzz",
						Destination: "/etc/fake/fake.conf",
						Type:        "config",
					},
				},
			},
		}),
		ioutil.Discard,
	)
	require.EqualError(t, err, fmt.Sprintf("matching \"%s\": file does not exist", filepath.ToSlash(abs)))
}

func TestDebNoFiles(t *testing.T) {
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
		ioutil.Discard,
	)
	require.NoError(t, err)
}

func TestDebNoInfo(t *testing.T) {
	err := Default.Package(nfpm.WithDefaults(&nfpm.Info{}), ioutil.Discard)
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
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/etc/fake",
					Type:        "config",
				},
			},
		},
	})
	err := info.Validate()
	require.NoError(t, err)
	out := conffiles(info)
	require.Equal(t, "/etc/fake\n", string(out), "should have a trailing empty line")
}

func TestPathsToCreate(t *testing.T) {
	for filePath, parts := range map[string][]string{
		"/usr/share/doc/whatever/foo.md": {"usr", "usr/share", "usr/share/doc", "usr/share/doc/whatever"},
		"/var/moises":                    {"var"},
		"/":                              {},
	} {
		parts := parts
		filePath := filePath
		t.Run(fmt.Sprintf("path: '%s'", filePath), func(t *testing.T) {
			require.Equal(t, parts, pathsToCreate(filePath))
		})
	}
}

func TestMinimalFields(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "minimal",
			Arch:        "arm64",
			Description: "Minimal does nothing",
			Priority:    "extra",
			Version:     "1.0.0",
			Section:     "default",
		}),
	}))
	golden := "testdata/minimal.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestDebEpoch(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
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
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestDebRules(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "lala",
			Arch:        "arm64",
			Description: "Has rules script",
			Priority:    "extra",
			Epoch:       "2",
			Version:     "1.2.0",
			Section:     "default",
			Overridables: nfpm.Overridables{
				Deb: nfpm.Deb{
					Scripts: nfpm.DebScripts{
						Rules: "foo.sh",
					},
				},
			},
		}),
	}))
	golden := "testdata/rules.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestMultilineFields(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "multiline",
			Arch:        "riscv64",
			Description: "This field is a\nmultiline field\nthat should work.",
			Priority:    "extra",
			Version:     "1.0.0",
			Section:     "default",
		}),
	}))
	golden := "testdata/multiline.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}

func TestDEBConventionalFileName(t *testing.T) {
	info := &nfpm.Info{
		Name: "testpkg",
		Arch: "all",
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
			Expected: fmt.Sprintf("%s_1.2.3_%s.deb", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "4", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3-4_%s.deb", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "4", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3~5-4_%s.deb", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3~5_%s.deb", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "1", Prerelease: "5", Metadata: "git",
			Expected: fmt.Sprintf("%s_1.2.3~5+git-1_%s.deb", info.Name, info.Arch),
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

func TestDebChangelogControl(t *testing.T) {
	info := &nfpm.Info{
		Name:        "changelog-test",
		Arch:        "amd64",
		Description: "This package has changelogs.",
		Version:     "1.0.0",
		Changelog:   "../testdata/changelog.yaml",
	}
	err := info.Validate()
	require.NoError(t, err)

	controlTarGz, err := createControl(0, []byte{}, info)
	require.NoError(t, err)

	controlChangelog := extractFileFromTar(t, inflate(t, "gz", controlTarGz), "changelog")

	goldenChangelog := readAndFormatAsDebChangelog(t, info.Changelog, info.Name)

	require.Equal(t, goldenChangelog, string(controlChangelog))
}

func TestDebNoChangelogControlWithoutChangelogConfigured(t *testing.T) {
	info := &nfpm.Info{
		Name:        "no-changelog-test",
		Arch:        "amd64",
		Description: "This package has explicitly no changelog.",
		Version:     "1.0.0",
	}
	err := info.Validate()
	require.NoError(t, err)

	controlTarGz, err := createControl(0, []byte{}, info)
	require.NoError(t, err)

	require.False(t, tarContains(t, inflate(t, "gz", controlTarGz), "changelog"))
}

func TestDebChangelogData(t *testing.T) {
	info := &nfpm.Info{
		Name:        "changelog-test",
		Arch:        "amd64",
		Description: "This package has changelogs.",
		Version:     "1.0.0",
		Changelog:   "../testdata/changelog.yaml",
	}
	err := info.Validate()
	require.NoError(t, err)

	dataTarball, _, _, dataTarballName, err := createDataTarball(info)
	require.NoError(t, err)

	changelogName := fmt.Sprintf("/usr/share/doc/%s/changelog.gz", info.Name)
	dataChangelogGz := extractFileFromTar(t,
		inflate(t, dataTarballName, dataTarball), changelogName)

	dataChangelog := inflate(t, "gz", dataChangelogGz)
	goldenChangelog := readAndFormatAsDebChangelog(t, info.Changelog, info.Name)

	require.Equal(t, goldenChangelog, string(dataChangelog))
}

func TestDebNoChangelogDataWithoutChangelogConfigured(t *testing.T) {
	info := &nfpm.Info{
		Name:        "no-changelog-test",
		Arch:        "amd64",
		Description: "This package has explicitly no changelog.",
		Version:     "1.0.0",
	}
	err := info.Validate()
	require.NoError(t, err)

	dataTarball, _, _, dataTarballName, err := createDataTarball(info)
	require.NoError(t, err)

	changelogName := fmt.Sprintf("/usr/share/doc/%s/changelog.gz", info.Name)

	require.False(t, tarContains(t, inflate(t, dataTarballName, dataTarball), changelogName))
}

func TestDebTriggers(t *testing.T) {
	info := &nfpm.Info{
		Name:        "no-triggers-test",
		Arch:        "amd64",
		Description: "This package has multiple triggers.",
		Version:     "1.0.0",
		Overridables: nfpm.Overridables{
			Deb: nfpm.Deb{
				Triggers: nfpm.DebTriggers{
					Interest:      []string{"trigger1", "trigger2"},
					InterestAwait: []string{"trigger3"},
					// InterestNoAwait omitted
					// Activate omitted
					ActivateAwait:   []string{"trigger4"},
					ActivateNoAwait: []string{"trigger5", "trigger6"},
				},
			},
		},
	}
	err := info.Validate()
	require.NoError(t, err)

	controlTarGz, err := createControl(0, []byte{}, info)
	require.NoError(t, err)

	controlTriggers := extractFileFromTar(t, inflate(t, "gz", controlTarGz), "triggers")

	goldenTriggers := createTriggers(info)

	require.Equal(t, string(goldenTriggers), string(controlTriggers))

	// check if specified triggers are included and also that
	// no remnants of triggers that were not specified are included
	require.True(t, bytes.Contains(controlTriggers,
		[]byte("interest trigger1\n")))
	require.True(t, bytes.Contains(controlTriggers,
		[]byte("interest trigger2\n")))
	require.True(t, bytes.Contains(controlTriggers,
		[]byte("interest-await trigger3\n")))
	require.False(t, bytes.Contains(controlTriggers,
		[]byte("interest-noawait ")))
	require.False(t, bytes.Contains(controlTriggers,
		[]byte("activate ")))
	require.True(t, bytes.Contains(controlTriggers,
		[]byte("activate-await trigger4\n")))
	require.True(t, bytes.Contains(controlTriggers,
		[]byte("activate-noawait trigger5\n")))
	require.True(t, bytes.Contains(controlTriggers,
		[]byte("activate-noawait trigger6\n")))
}

func TestDebNoTriggersInControlIfNoneProvided(t *testing.T) {
	info := &nfpm.Info{
		Name:        "no-triggers-test",
		Arch:        "amd64",
		Description: "This package has explicitly no triggers.",
		Version:     "1.0.0",
	}
	err := info.Validate()
	require.NoError(t, err)

	controlTarGz, err := createControl(0, []byte{}, info)
	require.NoError(t, err)

	require.False(t, tarContains(t, inflate(t, "gz", controlTarGz), "triggers"))
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
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/whatever.conf",
					Destination: configFilePath,
				},
				{
					Source:      symlinkTarget,
					Destination: symlink,
					Type:        "symlink",
				},
			},
		},
	}
	err := info.Validate()
	require.NoError(t, err)

	dataTarball, _, _, dataTarballName, err := createDataTarball(info)
	require.NoError(t, err)

	packagedSymlinkHeader := extractFileHeaderFromTar(t,
		inflate(t, dataTarballName, dataTarball), symlink)

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
			Type:        "symlink",
		},
	}
	info.Changelog = "../testdata/changelog.yaml"
	err := info.Validate()
	require.NoError(t, err)

	dataTarball, md5sums, instSize, tarballName, err := createDataTarball(info)
	require.NoError(t, err)
	testRelativePathPrefixInTar(t, inflate(t, tarballName, dataTarball))

	controlTarGz, err := createControl(instSize, md5sums, info)
	require.NoError(t, err)
	testRelativePathPrefixInTar(t, inflate(t, "gz", controlTarGz))
}

func TestMD5Sums(t *testing.T) {
	info := exampleInfo()
	info.Changelog = "../testdata/changelog.yaml"
	err := info.Validate()
	require.NoError(t, err)

	nFiles := 1
	for _, f := range info.Contents {
		if (f.Packager == "" || f.Packager == "deb") && f.Type != "dir" {
			nFiles++
		}
	}

	dataTarball, md5sums, instSize, tarballName, err := createDataTarball(info)
	require.NoError(t, err)

	controlTarGz, err := createControl(instSize, md5sums, info)
	require.NoError(t, err)

	md5sumsFile := extractFileFromTar(t, inflate(t, "gz", controlTarGz), "./md5sums")

	lines := strings.Split(strings.TrimRight(string(md5sumsFile), "\n"), "\n")
	require.Equal(t, nFiles, len(lines))

	dataTar := inflate(t, tarballName, dataTarball)

	for _, line := range lines {
		parts := strings.Fields(line)
		require.Equal(t, len(parts), 2)

		md5sum, fileName := parts[0], parts[1]

		digest := md5.New() // nolint:gosec
		_, err = digest.Write(extractFileFromTar(t, dataTar, fileName))
		require.NoError(t, err)
		require.Equal(t, md5sum, hex.EncodeToString(digest.Sum(nil)))
	}
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
			Type:        "dir",
			FileInfo: &files.ContentFileInfo{
				Owner: "test",
				Mode:  0o700,
			},
		},
		{
			Destination: "/etc/baz",
			Type:        "dir",
		},
		{
			Destination: "/usr/lib/something/somethingelse",
			Type:        "dir",
		},
	}

	require.NoError(t, info.Validate())

	deflatedDataTarball, _, _, dataTarballName, err := createDataTarball(info)
	require.NoError(t, err)
	dataTarball := inflate(t, dataTarballName, deflatedDataTarball)

	require.Equal(t, []string{
		"./etc/",
		"./etc/bar/",
		"./etc/baz/",
		"./usr/",
		"./usr/lib/",
		"./usr/lib/something/",
		"./usr/lib/something/somethingelse/",
		"./etc/bar/file",
		"./etc/foo/",
		"./etc/foo/file",
	}, getTree(t, dataTarball))

	// for debs all implicit or explicit directories are created in the tarball
	h := extractFileHeaderFromTar(t, dataTarball, "/etc")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/etc/foo")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	h = extractFileHeaderFromTar(t, dataTarball, "/etc/bar")
	require.Equal(t, h.Typeflag, byte(tar.TypeDir))
	require.Equal(t, h.Mode, int64(0o700))
	require.Equal(t, h.Uname, "test")
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
			Type:        "dir",
		},
		{
			Destination: "/etc/baz",
			Type:        "dir",
		},
	}

	require.NoError(t, info.Validate())

	deflatedDataTarball, _, _, dataTarballName, err := createDataTarball(info)
	require.NoError(t, err)
	dataTarball := inflate(t, dataTarballName, deflatedDataTarball)

	exists := map[string]bool{}

	tr := tar.NewReader(bytes.NewReader(dataTarball))
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

func TestDebsigsSignature(t *testing.T) {
	info := exampleInfo()
	info.Deb.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.Deb.Signature.KeyPassphrase = "hunter2"

	var deb bytes.Buffer
	err := Default.Package(info, &deb)
	require.NoError(t, err)

	debBinary := extractFileFromAr(t, deb.Bytes(), "debian-binary")
	controlTarGz := extractFileFromAr(t, deb.Bytes(), "control.tar.gz")
	dataTarball := extractFileFromAr(t, deb.Bytes(), findDataTarball(t, deb.Bytes()))
	signature := extractFileFromAr(t, deb.Bytes(), "_gpgorigin")

	message := io.MultiReader(bytes.NewReader(debBinary),
		bytes.NewReader(controlTarGz), bytes.NewReader(dataTarball))

	err = sign.PGPVerify(message, signature, "../internal/sign/testdata/pubkey.asc")
	require.NoError(t, err)
}

func TestDebsigsSignatureError(t *testing.T) {
	info := exampleInfo()
	info.Deb.Signature.KeyFile = "/does/not/exist"

	var deb bytes.Buffer
	err := Default.Package(info, &deb)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.True(t, errors.As(err, &expectedError))
}

func TestDpkgSigSignature(t *testing.T) {
	info := exampleInfo()
	info.Deb.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.Deb.Signature.KeyPassphrase = "hunter2"
	info.Deb.Signature.Method = "dpkg-sig"
	info.Deb.Signature.Signer = "bob McRobert"

	var deb bytes.Buffer
	err := Default.Package(info, &deb)
	require.NoError(t, err)

	signature := extractFileFromAr(t, deb.Bytes(), "_gpgbuilder")

	err = sign.PGPReadMessage(signature, "../internal/sign/testdata/pubkey.asc")
	require.NoError(t, err)
}

func TestDpkgSigSignatureError(t *testing.T) {
	info := exampleInfo()
	info.Deb.Signature.KeyFile = "/does/not/exist"
	info.Deb.Signature.Method = "dpkg-sig"

	var deb bytes.Buffer
	err := Default.Package(info, &deb)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.True(t, errors.As(err, &expectedError))
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
	require.NoError(t, info.Validate())

	dataTarball, _, _, tarballName, err := createDataTarball(info)
	require.NoError(t, err)

	expectedContent, err := ioutil.ReadFile("../testdata/{file}[")
	require.NoError(t, err)

	actualContent := extractFileFromTar(t, inflate(t, tarballName, dataTarball), "/test/{file}[")

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
			Type:        "dir",
			Destination: "/etc/foo",
		},
	}
	require.NoError(t, info.Validate())

	expected := map[string]bool{
		"./etc/":        true,
		"./etc/foo/":    true,
		"./etc/foo/bar": true,
	}

	dataTarball, _, _, tarballName, err := createDataTarball(info)
	require.NoError(t, err)

	contents := tarContents(t, inflate(t, tarballName, dataTarball))

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
			Type:        "dir",
			Destination: "/etc/foo",
		},
		{
			Type:        "dir",
			Destination: "/etc/foo/",
		},
	}
	require.NoError(t, info.Validate())

	_, _, _, _, err := createDataTarball(info)
	require.Error(t, err)
}

func TestCompressionAlgorithms(t *testing.T) {
	testCases := []struct {
		algorithm       string
		dataTarballName string
	}{
		{"gzip", "data.tar.gz"},
		{"", "data.tar.gz"}, // test current default
		{"xz", "data.tar.xz"},
		{"none", "data.tar"},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.algorithm, func(t *testing.T) {
			info := exampleInfo()
			info.Deb.Compression = testCase.algorithm

			var deb bytes.Buffer

			err := Default.Package(info, &deb)
			require.NoError(t, err)

			dataTarballName := findDataTarball(t, deb.Bytes())
			require.Equal(t, dataTarballName, testCase.dataTarballName)

			dataTarball := extractFileFromAr(t, deb.Bytes(), dataTarballName)
			dataTar := inflate(t, dataTarballName, dataTarball)

			for _, file := range info.Contents {
				tarContains(t, dataTar, file.Destination)
			}
		})
	}
}

func extractFileFromTar(tb testing.TB, tarFile []byte, filename string) []byte {
	tb.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) != path.Join("/", filename) {
			continue
		}

		fileContents, err := ioutil.ReadAll(tr)
		require.NoError(tb, err)

		return fileContents
	}

	tb.Fatalf("file %q does not exist in tar", filename)

	return nil
}

func tarContains(tb testing.TB, tarFile []byte, filename string) bool {
	tb.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) == path.Join("/", filename) { // nolint:gosec
			return true
		}
	}

	return false
}

func tarContents(tb testing.TB, tarFile []byte) []string {
	tb.Helper()

	contents := []string{}

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		contents = append(contents, hdr.Name)
	}

	return contents
}

func getTree(tb testing.TB, tarFile []byte) []string {
	tb.Helper()

	var result []string
	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		result = append(result, hdr.Name)
	}

	return result
}

func extractFileHeaderFromTar(tb testing.TB, tarFile []byte, filename string) *tar.Header {
	tb.Helper()

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) != path.Join("/", filename) { // nolint:gosec
			continue
		}

		return hdr
	}

	tb.Fatalf("file %q does not exist in tar", filename)

	return nil
}

func inflate(tb testing.TB, nameOrType string, data []byte) []byte {
	tb.Helper()

	ext := filepath.Ext(nameOrType)
	if ext == "" {
		ext = nameOrType
	} else {
		ext = strings.TrimPrefix(ext, ".")
	}

	dataReader := bytes.NewReader(data)

	var (
		inflateReadCloser io.ReadCloser
		err               error
	)

	switch ext {
	case "gz", "gzip":
		inflateReadCloser, err = gzip.NewReader(dataReader)
		require.NoError(tb, err)
	case "xz":
		r, err := xz.NewReader(dataReader, 0)
		require.NoError(tb, err)
		inflateReadCloser = io.NopCloser(r)
	case "tar", "": // no compression
		inflateReadCloser = io.NopCloser(dataReader)
	default:
		tb.Fatalf("invalid inflation type: %s", ext)
	}

	inflatedData, err := ioutil.ReadAll(inflateReadCloser)
	require.NoError(tb, err)

	err = inflateReadCloser.Close()
	require.NoError(tb, err)

	return inflatedData
}

func readAndFormatAsDebChangelog(tb testing.TB, changelogFileName, packageName string) string {
	tb.Helper()

	changelogEntries, err := chglog.Parse(changelogFileName)
	require.NoError(tb, err)

	tpl, err := chglog.DebTemplate()
	require.NoError(tb, err)

	debChangelog, err := chglog.FormatChangelog(&chglog.PackageChangeLog{
		Name:    packageName,
		Entries: changelogEntries,
	}, tpl)
	require.NoError(tb, err)

	return strings.TrimSpace(debChangelog) + "\n"
}

func findDataTarball(tb testing.TB, arFile []byte) string {
	tb.Helper()

	tr := ar.NewReader(bytes.NewReader(arFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if strings.HasPrefix(path.Join("/", hdr.Name), "/data.tar") {
			return hdr.Name
		}
	}

	tb.Fatalf("data taball does not exist in ar")

	return ""
}

func extractFileFromAr(tb testing.TB, arFile []byte, filename string) []byte {
	tb.Helper()

	tr := ar.NewReader(bytes.NewReader(arFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)

		if path.Join("/", hdr.Name) != path.Join("/", filename) {
			continue
		}

		fileContents, err := ioutil.ReadAll(tr)
		require.NoError(tb, err)

		return fileContents
	}

	tb.Fatalf("file %q does not exist in ar", filename)

	return nil
}

func TestEmptyButRequiredDebFields(t *testing.T) {
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
	for k := range archToDebian {
		t.Run(k, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = k
			info = ensureValidArch(info)
			require.Equal(t, archToDebian[k], info.Arch)
		})
	}

	t.Run("override", func(t *testing.T) {
		info := exampleInfo()
		info.Deb.Arch = "foo64"
		info = ensureValidArch(info)
		require.Equal(t, "foo64", info.Arch)
	})
}

func TestFields(t *testing.T) {
	var w bytes.Buffer
	require.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "foo",
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "v1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Overridables: nfpm.Overridables{
				Deb: nfpm.Deb{
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
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0o600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	require.Equal(t, string(bts), w.String())
}
