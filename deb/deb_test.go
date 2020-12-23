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
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/blakesmith/ar"
	"github.com/goreleaser/chglog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
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
			},
			EmptyFolders: []string{
				"/var/log/whatever",
				"/usr/share/whatever",
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
			var err = Default.Package(info, ioutil.Discard)
			assert.NoError(t, err)
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
	var err = Default.Package(info, ioutil.Discard)
	assert.NoError(t, err)
}

func TestDebVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	var buf bytes.Buffer
	var err = writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	var v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0", v)
}

func TestDebVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "1"
	var buf bytes.Buffer
	var err = writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	var v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0-1", v)
}

func TestDebVersionWithPrerelease(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Prerelease = "1"
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	assert.Equal(t, "1.0.0~1", v)
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
	assert.Equal(t, "1.0.0-2~rc1", v)
}

func TestDebVersionWithVersionMetadata(t *testing.T) {
	var buf bytes.Buffer

	info := exampleInfo()
	info.Version = "1.0.0+meta" //nolint:golint,goconst
	info.VersionMetadata = ""
	err := writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v := extractDebVersion(&buf)
	assert.Equal(t, "1.0.0+meta", v)

	buf.Reset()

	info.Version = "1.0.0" //nolint:golint,goconst
	info.VersionMetadata = "meta"
	err = writeControl(&buf, controlData{info, 0})
	require.NoError(t, err)
	v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0+meta", v)

	buf.Reset()

	info.Version = "1.0.0+foo" //nolint:golint,goconst
	info.Prerelease = "alpha"
	info.VersionMetadata = "meta"
	err = writeControl(&buf, controlData{nfpm.WithDefaults(info), 0})
	require.NoError(t, err)
	v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0~alpha+meta", v)
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(),
		InstalledSize: 10,
	}))
	var golden = "testdata/control.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestSpecialFiles(t *testing.T) {
	var w bytes.Buffer
	var out = tar.NewWriter(&w)
	filePath := "testdata/templates.golden"
	assert.Error(t, newFilePathInsideTarGz(out, "doesnotexit", "templates", 0644))
	require.NoError(t, newFilePathInsideTarGz(out, filePath, "templates", 0644))
	var in = tar.NewReader(&w)
	header, err := in.Next()
	require.NoError(t, err)
	assert.Equal(t, "templates", header.FileInfo().Name())
	mode, err := strconv.ParseInt("0644", 8, 64)
	require.NoError(t, err)
	assert.Equal(t, int64(header.FileInfo().Mode()), mode)
	data, err := ioutil.ReadAll(in)
	require.NoError(t, err)
	org, err := ioutil.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, data, org)
}

func TestNoJoinsControl(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
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
	var golden = "testdata/control2.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestDebFileDoesNotExist(t *testing.T) {
	var err = Default.Package(
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
	assert.EqualError(t, err, "matching \"../testdata/whatever.confzzz\": file does not exist")
}

func TestDebNoFiles(t *testing.T) {
	var err = Default.Package(
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
	assert.NoError(t, err)
}

func TestDebNoInfo(t *testing.T) {
	var err = Default.Package(nfpm.WithDefaults(&nfpm.Info{}), ioutil.Discard)
	assert.Error(t, err)
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
	assert.Equal(t, "/etc/fake\n", string(out), "should have a trailing empty line")
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
			assert.Equal(t, parts, pathsToCreate(filePath))
		})
	}
}

func TestMinimalFields(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "minimal",
			Arch:        "arm64",
			Description: "Minimal does nothing",
			Priority:    "extra",
			Version:     "1.0.0",
			Section:     "default",
		}),
	}))
	var golden = "testdata/minimal.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestDebEpoch(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
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
	var golden = "testdata/withepoch.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestDebRules(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
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
	var golden = "testdata/rules.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestMultilineFields(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(&nfpm.Info{
			Name:        "multiline",
			Arch:        "riscv64",
			Description: "This field is a\nmultiline field\nthat should work.",
			Priority:    "extra",
			Version:     "1.0.0",
			Section:     "default",
		}),
	}))
	var golden = "testdata/multiline.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
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
		{Version: "1.2.3", Release: "", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "4", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3-4_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "4", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3-4~5_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s_1.2.3~5_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "1", Prerelease: "5", Metadata: "git",
			Expected: fmt.Sprintf("%s_1.2.3-1~5+git_%s.deb", info.Name, info.Arch)},
	}

	for _, testCase := range testCases {
		info.Version = testCase.Version
		info.Release = testCase.Release
		info.Prerelease = testCase.Prerelease
		info.VersionMetadata = testCase.Metadata

		assert.Equal(t, testCase.Expected, Default.ConventionalFileName(info))
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

	controlChangelog, err := extractFileFromTarGz(controlTarGz, "changelog")
	require.NoError(t, err)

	goldenChangelog, err := readAndFormatAsDebChangelog(info.Changelog, info.Name)
	require.NoError(t, err)

	assert.Equal(t, goldenChangelog, string(controlChangelog))
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

	_, err = extractFileFromTarGz(controlTarGz, "changelog")
	assert.EqualError(t, err, os.ErrNotExist.Error())
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

	dataTarGz, _, _, err := createDataTarGz(info)
	require.NoError(t, err)

	changelogName := fmt.Sprintf("/usr/share/doc/%s/changelog.gz", info.Name)
	dataChangelogGz, err := extractFileFromTarGz(dataTarGz, changelogName)
	require.NoError(t, err)

	dataChangelog, err := gzipInflate(dataChangelogGz)
	require.NoError(t, err)

	goldenChangelog, err := readAndFormatAsDebChangelog(info.Changelog, info.Name)
	require.NoError(t, err)

	assert.Equal(t, goldenChangelog, string(dataChangelog))
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

	dataTarGz, _, _, err := createDataTarGz(info)
	require.NoError(t, err)

	changelogName := fmt.Sprintf("/usr/share/doc/%s/changelog.gz", info.Name)
	_, err = extractFileFromTarGz(dataTarGz, changelogName)
	assert.EqualError(t, err, os.ErrNotExist.Error())
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

	controlTriggers, err := extractFileFromTarGz(controlTarGz, "triggers")
	require.NoError(t, err)

	goldenTriggers := createTriggers(info)

	assert.Equal(t, string(goldenTriggers), string(controlTriggers))

	// check if specified triggers are included and also that
	// no remnants of triggers that were not specified are included
	assert.True(t, bytes.Contains(controlTriggers,
		[]byte("interest trigger1\n")))
	assert.True(t, bytes.Contains(controlTriggers,
		[]byte("interest trigger2\n")))
	assert.True(t, bytes.Contains(controlTriggers,
		[]byte("interest-await trigger3\n")))
	assert.False(t, bytes.Contains(controlTriggers,
		[]byte("interest-noawait ")))
	assert.False(t, bytes.Contains(controlTriggers,
		[]byte("activate ")))
	assert.True(t, bytes.Contains(controlTriggers,
		[]byte("activate-await trigger4\n")))
	assert.True(t, bytes.Contains(controlTriggers,
		[]byte("activate-noawait trigger5\n")))
	assert.True(t, bytes.Contains(controlTriggers,
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

	_, err = extractFileFromTarGz(controlTarGz, "triggers")
	assert.EqualError(t, err, os.ErrNotExist.Error())
}

func TestSymlinkInFiles(t *testing.T) {
	var (
		symlinkTarget  = "../testdata/whatever.conf"
		packagedTarget = "/etc/fake/whatever.conf"
	)

	info := &nfpm.Info{
		Name:        "symlink-in-files",
		Arch:        "amd64",
		Description: "This package's config references a file via symlink.",
		Version:     "1.0.0",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      symlinkTo(t, symlinkTarget),
					Destination: packagedTarget,
				},
			},
		},
	}
	err := info.Validate()
	require.NoError(t, err)

	realSymlinkTarget, err := ioutil.ReadFile(symlinkTarget)
	require.NoError(t, err)

	dataTarGz, _, _, err := createDataTarGz(info)
	require.NoError(t, err)

	packagedSymlinkTarget, err := extractFileFromTarGz(dataTarGz, packagedTarget)
	require.NoError(t, err)

	assert.Equal(t, string(realSymlinkTarget), string(packagedSymlinkTarget))
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

	dataTarGz, _, _, err := createDataTarGz(info)
	require.NoError(t, err)

	packagedSymlinkHeader, err := extractFileHeaderFromTarGz(dataTarGz, symlink)
	require.NoError(t, err)

	assert.Equal(t, symlink, path.Join("/", packagedSymlinkHeader.Name)) // nolint:gosec
	assert.Equal(t, uint8(tar.TypeSymlink), packagedSymlinkHeader.Typeflag)
	assert.Equal(t, symlinkTarget, packagedSymlinkHeader.Linkname)
}

func TestEnsureRelativePrefixInTarGzFiles(t *testing.T) {
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

	dataTarGz, md5sums, instSize, err := createDataTarGz(info)
	require.NoError(t, err)
	testRelativePathPrefixInTarGzFiles(t, dataTarGz)

	controlTarGz, err := createControl(instSize, md5sums, info)
	require.NoError(t, err)
	testRelativePathPrefixInTarGzFiles(t, controlTarGz)
}

func TestMD5Sums(t *testing.T) {
	info := exampleInfo()
	info.Changelog = "../testdata/changelog.yaml"
	err := info.Validate()
	require.NoError(t, err)

	nFiles := 1
	for _, f := range info.Contents {
		if f.Packager == "" || f.Packager == "deb" {
			nFiles++
		}
	}

	dataTarGz, md5sums, instSize, err := createDataTarGz(info)
	require.NoError(t, err)

	controlTarGz, err := createControl(instSize, md5sums, info)
	require.NoError(t, err)

	md5sumsFile, err := extractFileFromTarGz(controlTarGz, "./md5sums")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(string(md5sumsFile), "\n"), "\n")
	require.Equal(t, nFiles, len(lines))

	for _, line := range lines {
		parts := strings.Fields(line)
		require.Equal(t, len(parts), 2)

		md5sum, fileName := parts[0], parts[1]

		fileContent, err := extractFileFromTarGz(dataTarGz, fileName)
		require.NoError(t, err)

		digest := md5.New() // nolint:gosec
		_, err = digest.Write(fileContent)
		require.NoError(t, err)
		assert.Equal(t, md5sum, hex.EncodeToString(digest.Sum(nil)))
	}
}

func testRelativePathPrefixInTarGzFiles(t *testing.T, tarGzFile []byte) {
	tarFile, err := gzipInflate(tarGzFile)
	require.NoError(t, err)

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(hdr.Name, "./"), "%s does not start with './'", hdr.Name)
	}
}

func TestDebsigsSignature(t *testing.T) {
	info := exampleInfo()
	info.Deb.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.Deb.Signature.KeyPassphrase = "hunter2"

	var deb bytes.Buffer
	err := Default.Package(info, &deb)
	require.NoError(t, err)

	debBinary, err := extractFileFromAr(deb.Bytes(), "debian-binary")
	require.NoError(t, err)

	controlTarGz, err := extractFileFromAr(deb.Bytes(), "control.tar.gz")
	require.NoError(t, err)

	dataTarGz, err := extractFileFromAr(deb.Bytes(), "data.tar.gz")
	require.NoError(t, err)

	signature, err := extractFileFromAr(deb.Bytes(), "_gpgorigin")
	require.NoError(t, err)

	message := io.MultiReader(bytes.NewReader(debBinary),
		bytes.NewReader(controlTarGz), bytes.NewReader(dataTarGz))

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

	dataTarGz, _, _, err := createDataTarGz(info)
	require.NoError(t, err)

	expectedContent, err := ioutil.ReadFile("../testdata/{file}[")
	require.NoError(t, err)

	actualContent, err := extractFileFromTarGz(dataTarGz, "/test/{file}[")
	require.NoError(t, err)

	assert.Equal(t, expectedContent, actualContent)
}

func extractFileFromTarGz(tarGzFile []byte, filename string) ([]byte, error) {
	tarFile, err := gzipInflate(tarGzFile)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		if path.Join("/", hdr.Name) != path.Join("/", filename) { // nolint:gosec
			continue
		}

		fileContents, err := ioutil.ReadAll(tr)
		if err != nil {
			return nil, err
		}

		return fileContents, nil
	}

	return nil, os.ErrNotExist
}

func extractFileHeaderFromTarGz(tarGzFile []byte, filename string) (*tar.Header, error) {
	tarFile, err := gzipInflate(tarGzFile)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		if path.Join("/", hdr.Name) != path.Join("/", filename) { // nolint:gosec
			continue
		}

		return hdr, nil
	}

	return nil, os.ErrNotExist
}

func gzipInflate(data []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	inflatedData, err := ioutil.ReadAll(gzr)
	if err != nil {
		return nil, err
	}

	if err = gzr.Close(); err != nil {
		return nil, err
	}

	return inflatedData, nil
}

func readAndFormatAsDebChangelog(changelogFileName, packageName string) (string, error) {
	changelogEntries, err := chglog.Parse(changelogFileName)
	if err != nil {
		return "", err
	}

	tpl, err := chglog.DebTemplate()
	if err != nil {
		return "", err
	}

	debChangelog, err := chglog.FormatChangelog(&chglog.PackageChangeLog{
		Name:    packageName,
		Entries: changelogEntries,
	}, tpl)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(debChangelog) + "\n", nil
}

func symlinkTo(tb testing.TB, fileName string) string {
	target, err := filepath.Abs(fileName)
	assert.NoError(tb, err)

	symlinkName := filepath.Join(tb.TempDir(), "symlink")
	err = os.Symlink(target, symlinkName)
	assert.NoError(tb, err)

	return files.ToNixPath(symlinkName)
}

func extractFileFromAr(arFile []byte, filename string) ([]byte, error) {
	tr := ar.NewReader(bytes.NewReader(arFile))
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		if path.Join("/", hdr.Name) != path.Join("/", filename) {
			continue
		}

		fileContents, err := ioutil.ReadAll(tr)
		if err != nil {
			return nil, err
		}

		return fileContents, nil
	}

	return nil, os.ErrNotExist
}
