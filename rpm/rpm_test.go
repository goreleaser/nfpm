package rpm

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sassoftware/go-rpmutils"
	"github.com/sassoftware/go-rpmutils/cpio"
	"github.com/stretchr/testify/assert"

	"github.com/goreleaser/chglog"

	"github.com/goreleaser/nfpm"
)

const (
	tagVersion     = 0x03e9 // 1001
	tagRelease     = 0x03ea // 1002
	tagEpoch       = 0x03eb // 1003
	tagSummary     = 0x03ec // 1004
	tagDescription = 0x03ed // 1005
	tagGroup       = 0x03f8 // 1016
	tagPrein       = 0x03ff // 1023
	tagPostin      = 0x0400 // 1024
	tagPreun       = 0x0401 // 1025
	tagPostun      = 0x0402 // 1026
)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "1.0.0",
		Section:     "default",
		Homepage:    "http://carlosbecker.com",
		Vendor:      "nope",
		License:     "MIT",
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
				"../testdata/fake": "/usr/local/bin/fake",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.conf": "/etc/fake/fake.conf",
			},
			EmptyFolders: []string{
				"/var/log/whatever",
				"/usr/share/whatever",
			},
			Scripts: nfpm.Scripts{
				PreInstall:  "../testdata/scripts/preinstall.sh",
				PostInstall: "../testdata/scripts/postinstall.sh",
				PreRemove:   "../testdata/scripts/preremove.sh",
				PostRemove:  "../testdata/scripts/postremove.sh",
			},
		},
	})
}

func TestRPM(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	defer func() {
		_ = f.Close()
		err = os.Remove(f.Name())
		assert.NoError(t, err)
	}()

	assert.NoError(t, Default.Package(exampleInfo(), f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0600) //nolint:gosec
	assert.NoError(t, err)
	rpm, err := rpmutils.ReadRpm(file)
	assert.NoError(t, err)

	version, err := rpm.Header.GetString(tagVersion)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", version)

	release, err := rpm.Header.GetString(tagRelease)
	assert.NoError(t, err)
	assert.Equal(t, "1", release)

	epoch, err := rpm.Header.Get(tagEpoch)
	assert.NoError(t, err)
	epochUint32, ok := epoch.([]uint32)
	assert.Len(t, epochUint32, 1)
	assert.True(t, ok)
	assert.Equal(t, uint32(0), epochUint32[0])

	group, err := rpm.Header.GetString(tagGroup)
	assert.NoError(t, err)
	assert.Equal(t, "Development/Tools", group)

	summary, err := rpm.Header.GetString(tagSummary)
	assert.NoError(t, err)
	assert.Equal(t, "Foo does things", summary)

	description, err := rpm.Header.GetString(tagDescription)
	assert.NoError(t, err)
	assert.Equal(t, "Foo does things", description)
}

func TestWithRPMTags(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	defer func() {
		_ = f.Close()
		err = os.Remove(f.Name())
		assert.NoError(t, err)
	}()

	var info = exampleInfo()
	info.Release = "3"
	info.Epoch = "42"
	info.RPM = nfpm.RPM{
		Group: "default",
	}
	info.Description = "first line\nsecond line\nthird line"
	assert.NoError(t, Default.Package(info, f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0600) //nolint:gosec
	assert.NoError(t, err)

	rpm, err := rpmutils.ReadRpm(file)
	assert.NoError(t, err)

	version, err := rpm.Header.GetString(tagVersion)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", version)

	release, err := rpm.Header.GetString(tagRelease)
	assert.NoError(t, err)
	assert.Equal(t, "3", release)

	epoch, err := rpm.Header.Get(tagEpoch)
	assert.NoError(t, err)
	epochUint32, ok := epoch.([]uint32)
	assert.Len(t, epochUint32, 1)
	assert.True(t, ok)
	assert.Equal(t, uint32(42), epochUint32[0])

	group, err := rpm.Header.GetString(tagGroup)
	assert.NoError(t, err)
	assert.Equal(t, "default", group)

	summary, err := rpm.Header.GetString(tagSummary)
	assert.NoError(t, err)
	assert.Equal(t, "first line", summary)

	description, err := rpm.Header.GetString(tagDescription)
	assert.NoError(t, err)
	assert.Equal(t, info.Description, description)
}

func TestRPMVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "1", meta.Release)
}

func TestRPMVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "2"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "2", meta.Release)
}

func TestRPMVersionWithPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Prerelease = "rc1"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "0.1.rc1", meta.Release)
}

func TestRPMVersionWithReleaseAndPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "0.2"
	info.Prerelease = "rc1"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "0.2.rc1", meta.Release)
}

func TestWithInvalidEpoch(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	defer func() {
		_ = f.Close()
		err = os.Remove(f.Name())
		assert.NoError(t, err)
	}()

	var info = exampleInfo()
	info.Release = "3"
	info.Epoch = "-1"
	info.RPM = nfpm.RPM{
		Group: "default",
	}
	info.Description = "first line\nsecond line\nthird line"
	assert.Error(t, Default.Package(info, f))
}

func TestRPMScripts(t *testing.T) {
	info := exampleInfo()
	f, err := ioutil.TempFile(".", fmt.Sprintf("%s-%s-*.rpm", info.Name, info.Version))
	defer func() {
		_ = f.Close()
		err = os.Remove(f.Name())
		assert.NoError(t, err)
	}()
	assert.NoError(t, err)
	err = Default.Package(info, f)
	assert.NoError(t, err)
	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0600) //nolint:gosec
	assert.NoError(t, err)
	rpm, err := rpmutils.ReadRpm(file)
	assert.NoError(t, err)

	data, err := rpm.Header.GetString(tagPrein)
	assert.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Preinstall" > /dev/null
`, data, "Preinstall script does not match")

	data, err = rpm.Header.GetString(tagPreun)
	assert.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Preremove" > /dev/null
`, data, "Preremove script does not match")

	data, err = rpm.Header.GetString(tagPostin)
	assert.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Postinstall" > /dev/null
`, data, "Postinstall script does not match")

	data, err = rpm.Header.GetString(tagPostun)
	assert.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Postremove" > /dev/null
`, data, "Postremove script does not match")
}

func TestRPMFileDoesNotExist(t *testing.T) {
	info := exampleInfo()
	info.Files = map[string]string{
		"../testdata/": "/usr/local/bin/fake",
	}
	info.ConfigFiles = map[string]string{
		"../testdata/whatever.confzzz": "/etc/fake/fake.conf",
	}
	var err = Default.Package(info, ioutil.Discard)
	assert.EqualError(t, err, "../testdata/whatever.confzzz: file does not exist")
}

func TestRPMMultiArch(t *testing.T) {
	info := exampleInfo()

	for k := range archToRPM {
		info.Arch = k
		info = ensureValidArch(info)
		assert.Equal(t, archToRPM[k], info.Arch)
	}
}

func TestRPMConventionalFileName(t *testing.T) {
	info := &nfpm.Info{
		Name: "testpkg",
		Arch: "noarch",
	}

	testCases := []struct {
		Version    string
		Release    string
		Prerelease string
		Expected   string
	}{
		{Version: "1.2.3", Release: "", Prerelease: "",
			Expected: fmt.Sprintf("%s-1.2.3.%s.rpm", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "4", Prerelease: "",
			Expected: fmt.Sprintf("%s-1.2.3-4.%s.rpm", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "4", Prerelease: "5",
			Expected: fmt.Sprintf("%s-1.2.3-4~5.%s.rpm", info.Name, info.Arch)},
		{Version: "1.2.3", Release: "", Prerelease: "5",
			Expected: fmt.Sprintf("%s-1.2.3~5.%s.rpm", info.Name, info.Arch)},
	}

	for _, testCase := range testCases {
		info.Version = testCase.Version
		info.Release = testCase.Release
		info.Prerelease = testCase.Prerelease

		assert.Equal(t, testCase.Expected, Default.ConventionalFileName(info))
	}
}

func TestRPMChangelog(t *testing.T) {
	info := exampleInfo()
	info.Changelog = "../testdata/changelog.yaml"

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	assert.NoError(t, err)

	rpm, err := rpmutils.ReadRpm(bytes.NewReader(rpmFileBuffer.Bytes()))
	assert.NoError(t, err)

	changelog, err := chglog.Parse(info.Changelog)
	assert.NoError(t, err)

	_times, err := rpm.Header.Get(tagChangelogTime)
	assert.NoError(t, err)
	times, ok := _times.([]uint32)
	assert.True(t, ok)
	assert.Equal(t, len(changelog), len(times))

	_titles, err := rpm.Header.Get(tagChangelogName)
	assert.NoError(t, err)
	titles, ok := _titles.([]string)
	assert.True(t, ok)
	assert.Equal(t, len(changelog), len(titles))

	_notes, err := rpm.Header.Get(tagChangelogText)
	assert.NoError(t, err)
	allNotes, ok := _notes.([]string)
	assert.True(t, ok)
	assert.Equal(t, len(changelog), len(allNotes))

	for i, entry := range changelog {
		timestamp := time.Unix(int64(times[i]), 0).UTC()
		title := titles[i]
		notes := strings.Split(allNotes[i], "\n")

		assert.Equal(t, entry.Date, timestamp)
		assert.True(t, strings.Contains(title, entry.Packager))
		assert.True(t, strings.Contains(title, entry.Semver))
		assert.Equal(t, len(entry.Changes), len(notes))

		for j, change := range entry.Changes {
			assert.True(t, strings.Contains(notes[j], change.Note))
		}
	}
}

func TestRPMNoChangelogTagsWithoutChangelogConfigured(t *testing.T) {
	info := exampleInfo()

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	assert.NoError(t, err)

	rpm, err := rpmutils.ReadRpm(bytes.NewReader(rpmFileBuffer.Bytes()))
	assert.NoError(t, err)

	_, err = rpm.Header.Get(tagChangelogTime)
	assert.Error(t, err)

	_, err = rpm.Header.Get(tagChangelogName)
	assert.Error(t, err)

	_, err = rpm.Header.Get(tagChangelogText)
	assert.Error(t, err)
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
			Files: map[string]string{
				symlinkTo(t, symlinkTarget): packagedTarget,
			},
		},
	}

	realSymlinkTarget, err := ioutil.ReadFile(symlinkTarget)
	assert.NoError(t, err)

	var rpmFileBuffer bytes.Buffer
	err = Default.Package(info, &rpmFileBuffer)
	assert.NoError(t, err)

	packagedSymlinkTarget, err := extractFileFromRpm(rpmFileBuffer.Bytes(), packagedTarget)
	assert.NoError(t, err)

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
			Files: map[string]string{
				"../testdata/whatever.conf": configFilePath,
			},
			Symlinks: map[string]string{
				symlink: symlinkTarget,
			},
		},
	}

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	assert.NoError(t, err)

	packagedSymlinkHeader, err := extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), symlink)
	assert.NoError(t, err)

	packagedSymlink, err := extractFileFromRpm(rpmFileBuffer.Bytes(), symlink)
	assert.NoError(t, err)

	assert.Equal(t, symlink, packagedSymlinkHeader.Filename())
	assert.Equal(t, cpio.S_ISLNK, packagedSymlinkHeader.Mode())
	assert.Equal(t, symlinkTarget, string(packagedSymlink))
}

func extractFileFromRpm(rpm []byte, filename string) ([]byte, error) {
	rpmFile, err := rpmutils.ReadRpm(bytes.NewReader(rpm))
	if err != nil {
		return nil, err
	}

	pr, err := rpmFile.PayloadReader()
	if err != nil {
		return nil, err
	}

	for {
		hdr, err := pr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		if hdr.Filename() != filename {
			continue
		}

		fileContents, err := ioutil.ReadAll(pr)
		if err != nil {
			return nil, err
		}

		return fileContents, nil
	}

	return nil, os.ErrNotExist
}

func extractFileHeaderFromRpm(rpm []byte, filename string) (*cpio.Cpio_newc_header, error) {
	rpmFile, err := rpmutils.ReadRpm(bytes.NewReader(rpm))
	if err != nil {
		return nil, err
	}

	pr, err := rpmFile.PayloadReader()
	if err != nil {
		return nil, err
	}

	for {
		hdr, err := pr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, err
		}

		if hdr.Filename() != filename {
			continue
		}

		return hdr, nil
	}

	return nil, os.ErrNotExist
}

func symlinkTo(tb testing.TB, fileName string) string {
	target, err := filepath.Abs(fileName)
	assert.NoError(tb, err)

	tempDir, err := ioutil.TempDir("", "nfpm_rpm_test")
	assert.NoError(tb, err)

	symlinkName := path.Join(tempDir, "symlink")
	err = os.Symlink(target, symlinkName)
	assert.NoError(tb, err)

	tb.Cleanup(func() {
		err = os.RemoveAll(tempDir)
		assert.NoError(tb, err)
	})

	return symlinkName
}
