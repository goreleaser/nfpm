package rpm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/chglog"
	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/sassoftware/go-rpmutils"
	"github.com/sassoftware/go-rpmutils/cpio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/openpgp"
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
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/usr/local/bin/fake",
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
			Scripts: nfpm.Scripts{
				PreInstall:  "../testdata/scripts/preinstall.sh",
				PostInstall: "../testdata/scripts/postinstall.sh",
				PreRemove:   "../testdata/scripts/preremove.sh",
				PostRemove:  "../testdata/scripts/postremove.sh",
			},
			RPM: nfpm.RPM{
				Scripts: nfpm.RPMScripts{
					PreTrans:  "../testdata/scripts/pretrans.sh",
					PostTrans: "../testdata/scripts/posttrans.sh",
				},
			},
		},
	})
}

func TestRPM(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	require.NoError(t, err)
	require.NoError(t, Default.Package(exampleInfo(), f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		assert.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	version, err := rpm.Header.GetString(rpmutils.VERSION)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", version)

	release, err := rpm.Header.GetString(rpmutils.RELEASE)
	require.NoError(t, err)
	assert.Equal(t, "1", release)

	epoch, err := rpm.Header.Get(rpmutils.EPOCH)
	require.NoError(t, err)
	epochUint32, ok := epoch.([]uint32)
	require.True(t, ok)
	assert.Len(t, epochUint32, 1)
	assert.Equal(t, uint32(0), epochUint32[0])

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	assert.Equal(t, "", group)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	assert.Equal(t, "Foo does things", summary)

	description, err := rpm.Header.GetString(rpmutils.DESCRIPTION)
	require.NoError(t, err)
	assert.Equal(t, "Foo does things", description)
}

func TestRPMGroup(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	require.NoError(t, err)
	info := exampleInfo()
	info.RPM.Group = "Unspecified"
	require.NoError(t, Default.Package(info, f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		assert.NoError(t, err)
	}()

	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	assert.Equal(t, "Unspecified", group)
}

func TestRPMSummary(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	require.NoError(t, err)

	customSummary := "This is my custom summary"
	info := exampleInfo()
	info.RPM.Group = "Unspecified"
	info.RPM.Summary = customSummary

	require.NoError(t, Default.Package(info, f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		assert.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	assert.Equal(t, customSummary, summary)
}

func TestWithRPMTags(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	require.NoError(t, err)

	info := exampleInfo()
	info.Release = "3"
	info.Epoch = "42"
	info.RPM = nfpm.RPM{
		Group: "default",
	}
	info.Description = "first line\nsecond line\nthird line"
	require.NoError(t, Default.Package(info, f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		assert.NoError(t, err)
	}()

	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	version, err := rpm.Header.GetString(rpmutils.VERSION)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", version)

	release, err := rpm.Header.GetString(rpmutils.RELEASE)
	require.NoError(t, err)
	assert.Equal(t, "3", release)

	epoch, err := rpm.Header.Get(rpmutils.EPOCH)
	require.NoError(t, err)
	epochUint32, ok := epoch.([]uint32)
	assert.Len(t, epochUint32, 1)
	assert.True(t, ok)
	assert.Equal(t, uint32(42), epochUint32[0])

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	assert.Equal(t, "default", group)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	assert.Equal(t, "first line", summary)

	description, err := rpm.Header.GetString(rpmutils.DESCRIPTION)
	require.NoError(t, err)
	assert.Equal(t, info.Description, description)
}

func TestRPMVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "1", meta.Release)
}

func TestRPMVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "2"
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "2", meta.Release)
}

func TestRPMVersionWithPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()

	info.Version = "1.0.0"
	info.Prerelease = "rc1" // nolint:goconst
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0~rc1", meta.Version)
	assert.Equal(t, "1", meta.Release)

	info.Version = "1.0.0~rc1"
	info.Prerelease = ""
	meta, err = buildRPMMeta(info)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0~rc1", meta.Version)
	assert.Equal(t, "1", meta.Release)
}

func TestRPMVersionWithReleaseAndPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()

	info.Version = "1.0.0"
	info.Release = "0.2"
	info.Prerelease = "rc1"
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0~rc1", meta.Version)
	assert.Equal(t, "0.2", meta.Release)

	info.Version = "1.0.0~rc1"
	info.Release = "0.2"
	info.Prerelease = ""
	meta, err = buildRPMMeta(info)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0~rc1", meta.Version)
	assert.Equal(t, "0.2", meta.Release)
}

func TestRPMVersionWithVersionMetadata(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()

	info.Version = "1.0.0+meta"
	info.VersionMetadata = ""
	meta, err := buildRPMMeta(nfpm.WithDefaults(info))
	require.NoError(t, err)
	assert.Equal(t, "1.0.0+meta", meta.Version)

	info.Version = "1.0.0"
	info.VersionMetadata = "meta"
	meta, err = buildRPMMeta(nfpm.WithDefaults(info))
	require.NoError(t, err)
	assert.Equal(t, "1.0.0+meta", meta.Version)
}

func TestWithInvalidEpoch(t *testing.T) {
	f, err := ioutil.TempFile("", "test.rpm")
	defer func() {
		_ = f.Close()
		err = os.Remove(f.Name())
		assert.NoError(t, err)
	}()

	info := exampleInfo()
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
	require.NoError(t, err)
	err = Default.Package(info, f)
	require.NoError(t, err)
	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		assert.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	data, err := rpm.Header.GetString(rpmutils.PREIN)
	require.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Preinstall" > /dev/null
`, data, "Preinstall script does not match")

	data, err = rpm.Header.GetString(rpmutils.PREUN)
	require.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Preremove" > /dev/null
`, data, "Preremove script does not match")

	data, err = rpm.Header.GetString(rpmutils.POSTIN)
	require.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Postinstall" > /dev/null
`, data, "Postinstall script does not match")

	data, err = rpm.Header.GetString(rpmutils.POSTUN)
	require.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Postremove" > /dev/null
`, data, "Postremove script does not match")

	rpmPreTransTag := 1151
	data, err = rpm.Header.GetString(rpmPreTransTag)
	require.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Pretrans" > /dev/null
`, data, "Pretrans script does not match")

	rpmPostTransTag := 1152
	data, err = rpm.Header.GetString(rpmPostTransTag)
	require.NoError(t, err)
	assert.Equal(t, `#!/bin/bash

echo "Posttrans" > /dev/null
`, data, "Posttrans script does not match")
}

func TestRPMFileDoesNotExist(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/fake",
			Destination: "/usr/local/bin/fake",
		},
		{
			Source:      "../testdata/whatever.confzzz",
			Destination: "/etc/fake/fake.conf",
			Type:        "config",
		},
	}
	abs, err := filepath.Abs("../testdata/whatever.confzzz")
	require.NoError(t, err)
	err = Default.Package(info, ioutil.Discard)
	assert.EqualError(t, err, fmt.Sprintf("matching \"%s\": file does not exist", filepath.ToSlash(abs)))
}

func TestRPMMultiArch(t *testing.T) {
	info := exampleInfo()

	for k := range archToRPM {
		info.Arch = k
		info = ensureValidArch(info)
		assert.Equal(t, archToRPM[k], info.Arch)
	}
}

func TestConfigNoReplace(t *testing.T) {
	var (
		buildConfigFile   = "../testdata/whatever.conf"
		packageConfigFile = "/etc/fake/fake.conf"
	)

	info := &nfpm.Info{
		Name:        "symlink-in-files",
		Arch:        "amd64",
		Description: "This package's config references a file via symlink.",
		Version:     "1.0.0",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      buildConfigFile,
					Destination: packageConfigFile,
					Type:        "config|noreplace",
				},
			},
		},
	}

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	assert.NoError(t, err)

	expectedConfigContent, err := ioutil.ReadFile(buildConfigFile)
	assert.NoError(t, err)

	packageConfigContent, err := extractFileFromRpm(rpmFileBuffer.Bytes(), packageConfigFile)
	assert.NoError(t, err)

	assert.Equal(t, expectedConfigContent, packageConfigContent)
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
		Metadata   string
	}{
		{
			Version: "1.2.3", Release: "", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s-1.2.3.%s.rpm", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "4", Prerelease: "", Metadata: "",
			Expected: fmt.Sprintf("%s-1.2.3-4.%s.rpm", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "4", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s-1.2.3~5-4.%s.rpm", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "", Prerelease: "5", Metadata: "",
			Expected: fmt.Sprintf("%s-1.2.3~5.%s.rpm", info.Name, info.Arch),
		},
		{
			Version: "1.2.3", Release: "1", Prerelease: "5", Metadata: "git",
			Expected: fmt.Sprintf("%s-1.2.3~5+git-1.%s.rpm", info.Name, info.Arch),
		},
	}

	for _, testCase := range testCases {
		info.Version = testCase.Version
		info.Release = testCase.Release
		info.Prerelease = testCase.Prerelease
		info.VersionMetadata = testCase.Metadata

		assert.Equal(t, testCase.Expected, Default.ConventionalFileName(info))
	}
}

func TestRPMChangelog(t *testing.T) {
	info := exampleInfo()
	info.Changelog = "../testdata/changelog.yaml"

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	rpm, err := rpmutils.ReadRpm(bytes.NewReader(rpmFileBuffer.Bytes()))
	require.NoError(t, err)

	changelog, err := chglog.Parse(info.Changelog)
	require.NoError(t, err)

	_times, err := rpm.Header.Get(tagChangelogTime)
	require.NoError(t, err)
	times, ok := _times.([]uint32)
	require.True(t, ok)
	assert.Equal(t, len(changelog), len(times))

	_titles, err := rpm.Header.Get(tagChangelogName)
	require.NoError(t, err)
	titles, ok := _titles.([]string)
	require.True(t, ok)
	assert.Equal(t, len(changelog), len(titles))

	_notes, err := rpm.Header.Get(tagChangelogText)
	require.NoError(t, err)
	allNotes, ok := _notes.([]string)
	require.True(t, ok)
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
	require.NoError(t, err)

	rpm, err := rpmutils.ReadRpm(bytes.NewReader(rpmFileBuffer.Bytes()))
	require.NoError(t, err)

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
			Contents: []*files.Content{
				{
					Source:      symlinkTo(t, symlinkTarget),
					Destination: packagedTarget,
				},
			},
		},
	}

	realSymlinkTarget, err := ioutil.ReadFile(symlinkTarget)
	require.NoError(t, err)

	var rpmFileBuffer bytes.Buffer
	err = Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	packagedSymlinkTarget, err := extractFileFromRpm(rpmFileBuffer.Bytes(), packagedTarget)
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

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	packagedSymlinkHeader, err := extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), symlink)
	require.NoError(t, err)

	packagedSymlink, err := extractFileFromRpm(rpmFileBuffer.Bytes(), symlink)
	require.NoError(t, err)

	assert.Equal(t, symlink, packagedSymlinkHeader.Filename())
	assert.Equal(t, cpio.S_ISLNK, packagedSymlinkHeader.Mode())
	assert.Equal(t, symlinkTarget, string(packagedSymlink))
}

func TestRPMSignature(t *testing.T) {
	info := exampleInfo()
	info.RPM.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.RPM.Signature.KeyPassphrase = "hunter2"

	pubkeyFileContent, err := ioutil.ReadFile("../internal/sign/testdata/pubkey.gpg")
	require.NoError(t, err)

	keyring, err := openpgp.ReadKeyRing(bytes.NewReader(pubkeyFileContent))
	require.NoError(t, err)
	require.NotNil(t, keyring, "cannot verify sigs with an empty keyring")

	var rpmBuffer bytes.Buffer
	err = Default.Package(info, &rpmBuffer)
	require.NoError(t, err)

	_, sigs, err := rpmutils.Verify(bytes.NewReader(rpmBuffer.Bytes()), keyring)
	require.NoError(t, err)
	require.Len(t, sigs, 2)
}

func TestRPMSignatureError(t *testing.T) {
	info := exampleInfo()
	info.RPM.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.RPM.Signature.KeyPassphrase = "wrongpass"

	var rpmBuffer bytes.Buffer
	err := Default.Package(info, &rpmBuffer)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.True(t, errors.As(err, &expectedError))
}

func TestRPMGhostFiles(t *testing.T) {
	filename := "/usr/lib/casper.a"

	info := &nfpm.Info{
		Name:        "rpm-ghost",
		Arch:        "amd64",
		Description: "This RPM contains ghost files.",
		Version:     "1.0.0",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Destination: filename,
					Type:        "ghost",
				},
			},
		},
	}

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	headerFiles, err := extraFileInfoSliceFromRpm(rpmFileBuffer.Bytes())
	require.NoError(t, err)

	type headerFileInfo struct {
		Name string
		Size int64
		Mode int
	}
	expected := []headerFileInfo{
		{filename, 0, cpio.S_ISREG | 0o644},
	}
	actual := make([]headerFileInfo, 0)
	for _, fileInfo := range headerFiles {
		actual = append(actual, headerFileInfo{fileInfo.Name(), fileInfo.Size(), fileInfo.Mode()})
	}
	assert.Equal(t, expected, actual)

	_, err = extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), filename)
	require.Error(t, err)

	_, err = extractFileFromRpm(rpmFileBuffer.Bytes(), filename)
	require.Error(t, err)
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

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	expectedContent, err := ioutil.ReadFile("../testdata/{file}[")
	require.NoError(t, err)

	actualContent, err := extractFileFromRpm(rpmFileBuffer.Bytes(), "/test/{file}[")
	require.NoError(t, err)

	assert.Equal(t, expectedContent, actualContent)
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
		if errors.Is(err, io.EOF) {
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

func extraFileInfoSliceFromRpm(rpm []byte) ([]rpmutils.FileInfo, error) {
	rpmFile, err := rpmutils.ReadRpm(bytes.NewReader(rpm))
	if err != nil {
		return nil, err
	}
	return rpmFile.Header.GetFiles()
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
		if errors.Is(err, io.EOF) {
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
	tb.Helper()
	target, err := filepath.Abs(fileName)
	assert.NoError(tb, err)

	symlinkName := path.Join(tb.TempDir(), "symlink")
	err = os.Symlink(target, symlinkName)
	assert.NoError(tb, err)

	return files.ToNixPath(symlinkName)
}
