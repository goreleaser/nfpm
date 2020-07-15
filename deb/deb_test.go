package deb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goreleaser/chglog"

	"github.com/goreleaser/nfpm"
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
			Files: map[string]string{
				"../testdata/fake":          "/usr/local/bin/fake",
				"../testdata/whatever.conf": "/usr/share/doc/fake/fake.txt",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.conf": "/etc/fake/fake.conf",
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
	assert.NoError(t, err)
	var v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0", v)
}

func TestDebVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "1"
	var buf bytes.Buffer
	var err = writeControl(&buf, controlData{info, 0})
	assert.NoError(t, err)
	var v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0-1", v)
}

func TestDebVersionWithPrerelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Prerelease = "1"
	var buf bytes.Buffer
	var err = writeControl(&buf, controlData{info, 0})
	assert.NoError(t, err)
	var v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0~1", v)
}

func TestDebVersionWithReleaseAndPrerelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "2"
	info.Prerelease = "rc1"
	var buf bytes.Buffer
	var err = writeControl(&buf, controlData{info, 0})
	assert.NoError(t, err)
	var v = extractDebVersion(&buf)
	assert.Equal(t, "1.0.0-2~rc1", v)
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
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestScripts(t *testing.T) {
	var w bytes.Buffer
	var out = tar.NewWriter(&w)
	path := "../testdata/scripts/preinstall.sh"
	assert.Error(t, newScriptInsideTarGz(out, "doesnotexit", "preinst"))
	assert.NoError(t, newScriptInsideTarGz(out, path, "preinst"))
	var in = tar.NewReader(&w)
	header, err := in.Next()
	assert.NoError(t, err)
	assert.Equal(t, "preinst", header.FileInfo().Name())
	mode, err := strconv.ParseInt("0755", 8, 64)
	assert.NoError(t, err)
	assert.Equal(t, int64(header.FileInfo().Mode()), mode)
	data, err := ioutil.ReadAll(in)
	assert.NoError(t, err)
	org, err := ioutil.ReadFile(path)
	assert.NoError(t, err)
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
				Depends:     []string{},
				Recommends:  []string{},
				Suggests:    []string{},
				Replaces:    []string{},
				Provides:    []string{},
				Conflicts:   []string{},
				Files:       map[string]string{},
				ConfigFiles: map[string]string{},
			},
		}),
		InstalledSize: 10,
	}))
	var golden = "testdata/control2.golden"
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0600))
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	assert.NoError(t, err)
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
				Files: map[string]string{
					"../testdata/": "/usr/local/bin/fake",
				},
				ConfigFiles: map[string]string{
					"../testdata/whatever.confzzz": "/etc/fake/fake.conf",
				},
			},
		}),
		ioutil.Discard,
	)
	assert.EqualError(t, err, "../testdata/whatever.confzzz: file does not exist")
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
	assert.NoError(t, err)
}

func TestConffiles(t *testing.T) {
	out := conffiles(&nfpm.Info{
		Overridables: nfpm.Overridables{
			ConfigFiles: map[string]string{
				"fake": "/etc/fake",
			},
		},
	})
	assert.Equal(t, "/etc/fake\n", string(out), "should have a trailing empty line")
}

func TestPathsToCreate(t *testing.T) {
	for path, parts := range map[string][]string{
		"/usr/share/doc/whatever/foo.md": {"usr", "usr/share", "usr/share/doc", "usr/share/doc/whatever"},
		"/var/moises":                    {"var"},
		"/":                              {},
	} {
		parts := parts
		path := path
		t.Run(fmt.Sprintf("path: '%s'", path), func(t *testing.T) {
			assert.Equal(t, parts, pathsToCreate(path))
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	}{
		{Version: "1.2.3", Release: "", Prerelease: "",
			Expected: fmt.Sprintf("%s_1.2.3_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "4", Prerelease: "",
			Expected: fmt.Sprintf("%s_1.2.3-4_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "4", Prerelease: "5",
			Expected: fmt.Sprintf("%s_1.2.3-4~5_%s.deb", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "", Prerelease: "5",
			Expected: fmt.Sprintf("%s_1.2.3~5_%s.deb", info.Name, info.Arch)},
	}

	for _, testCase := range testCases {
		info.Version = testCase.Version
		info.Release = testCase.Release
		info.Prerelease = testCase.Prerelease

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

	controlTarGz, err := createControl(0, []byte{}, info)
	assert.NoError(t, err)

	controlChangelog, err := extractFileFromTarGz(controlTarGz, "changelog")
	assert.NoError(t, err)

	goldenChangelog, err := readAndFormatAsDebChangelog(info.Changelog, info.Name)
	assert.NoError(t, err)

	assert.Equal(t, goldenChangelog, string(controlChangelog))
}

func TestDebNoChangelogControlWithoutChangelogConfigured(t *testing.T) {
	info := &nfpm.Info{
		Name:        "no-changelog-test",
		Arch:        "amd64",
		Description: "This package has explicitly no changelog.",
		Version:     "1.0.0",
	}

	controlTarGz, err := createControl(0, []byte{}, info)
	assert.NoError(t, err)

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

	dataTarGz, _, _, err := createDataTarGz(info)
	assert.NoError(t, err)

	changelogName := fmt.Sprintf("/usr/share/doc/%s/changelog.gz", info.Name)
	dataChangelogGz, err := extractFileFromTarGz(dataTarGz, changelogName)
	assert.NoError(t, err)

	dataChangelog, err := gzipInflate(dataChangelogGz)
	assert.NoError(t, err)

	goldenChangelog, err := readAndFormatAsDebChangelog(info.Changelog, info.Name)
	assert.NoError(t, err)

	assert.Equal(t, goldenChangelog, string(dataChangelog))
}

func TestDebNoChangelogDataWithoutChangelogConfigured(t *testing.T) {
	info := &nfpm.Info{
		Name:        "no-changelog-test",
		Arch:        "amd64",
		Description: "This package has explicitly no changelog.",
		Version:     "1.0.0",
	}

	dataTarGz, _, _, err := createDataTarGz(info)
	assert.NoError(t, err)

	changelogName := fmt.Sprintf("/usr/share/doc/%s/changelog.gz", info.Name)
	_, err = extractFileFromTarGz(dataTarGz, changelogName)
	assert.EqualError(t, err, os.ErrNotExist.Error())
}

func extractFileFromTarGz(tarGzFile []byte, filename string) ([]byte, error) {
	tarFile, err := gzipInflate(tarGzFile)
	if err != nil {
		return nil, err
	}

	tr := tar.NewReader(bytes.NewReader(tarFile))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		if hdr.Name != filename {
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
