package rpm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/caarlos0/go-rpmutils"
	"github.com/caarlos0/go-rpmutils/cpio"
	"github.com/goreleaser/chglog"
	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/sign"
	"github.com/stretchr/testify/require"
)

func exampleInfo() *nfpm.Info {
	return setDefaults(nfpm.WithDefaults(&nfpm.Info{
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
					Destination: "/usr/bin/fake",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/etc/fake/fake.conf",
					Type:        files.TypeConfig,
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
			Scripts: nfpm.Scripts{
				PreInstall:  "../testdata/scripts/preinstall.sh",
				PostInstall: "../testdata/scripts/postinstall.sh",
				PreRemove:   "../testdata/scripts/preremove.sh",
				PostRemove:  "../testdata/scripts/postremove.sh",
			},
			RPM: nfpm.RPM{
				Prefixes: []string{"/opt"},
				Scripts: nfpm.RPMScripts{
					PreTrans:  "../testdata/scripts/pretrans.sh",
					PostTrans: "../testdata/scripts/posttrans.sh",
				},
			},
		},
	}))
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".rpm", Default.ConventionalExtension())
}

func TestRPM(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test.rpm")
	require.NoError(t, err)
	require.NoError(t, Default.Package(exampleInfo(), f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		require.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	os, err := rpm.Header.GetString(rpmutils.OS)
	require.NoError(t, err)
	require.Equal(t, "linux", os)

	arch, err := rpm.Header.GetString(rpmutils.ARCH)
	require.NoError(t, err)
	require.Equal(t, archToRPM["amd64"], arch)

	version, err := rpm.Header.GetString(rpmutils.VERSION)
	require.NoError(t, err)
	require.Equal(t, "1.0.0-1", version)

	release, err := rpm.Header.GetString(rpmutils.RELEASE)
	require.NoError(t, err)
	require.Equal(t, "1", release)

	epoch, err := rpm.Header.Get(rpmutils.EPOCH)
	require.NoError(t, err)
	epochUint32, ok := epoch.([]uint32)
	require.True(t, ok)
	require.Len(t, epochUint32, 1)
	require.Equal(t, uint32(0), epochUint32[0])

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	require.Equal(t, "", group)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	require.Equal(t, "Foo does things", summary)

	description, err := rpm.Header.GetString(rpmutils.DESCRIPTION)
	require.NoError(t, err)
	require.Equal(t, "Foo does things", description)
}

func TestRPMPlatform(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test*.rpm")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	info := exampleInfo()
	info.Platform = "darwin"
	require.NoError(t, Default.Package(info, f))
}

func TestRPMGroup(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test.rpm")
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
		require.NoError(t, err)
	}()

	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	require.Equal(t, "Unspecified", group)
}

func TestRPMCompression(t *testing.T) {
	for _, compressor := range []string{"gzip", "lzma", "xz", "zstd"} {
		for _, level := range []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9} {
			compLevel := fmt.Sprintf("%v:%v", compressor, level)
			if strings.HasPrefix(compLevel, "xz:") {
				compLevel = "xz"
			}
			if strings.HasPrefix(compLevel, "lzma:") {
				compLevel = "lzma"
			}
			t.Run(compLevel, func(t *testing.T) {
				f, err := os.CreateTemp(t.TempDir(), "test.rpm")
				require.NoError(t, err)

				info := exampleInfo()
				info.RPM.Compression = compLevel

				require.NoError(t, Default.Package(info, f))
				file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
				require.NoError(t, err)
				defer func() {
					f.Close()
					file.Close()
					err = os.Remove(file.Name())
					require.NoError(t, err)
				}()
				rpm, err := rpmutils.ReadRpm(file)
				require.NoError(t, err)

				rpmCompressor, err := rpm.Header.GetString(rpmutils.PAYLOADCOMPRESSOR)
				require.NoError(t, err)
				require.Equal(t, compressor, rpmCompressor)
			})
			if compLevel == "xz" || compLevel == "lzma" {
				break
			}
		}
	}
}

func TestRPMSummary(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test.rpm")
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
		require.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	require.Equal(t, customSummary, summary)
}

func TestRPMPackager(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test.rpm")
	require.NoError(t, err)

	customPackager := "GoReleaser <staff@goreleaser.com>"
	info := exampleInfo()
	info.RPM.Group = "Unspecified"
	info.RPM.Packager = customPackager

	require.NoError(t, Default.Package(info, f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		require.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	packager, err := rpm.Header.GetString(rpmutils.PACKAGER)
	require.NoError(t, err)
	require.Equal(t, customPackager, packager)
}

func TestWithRPMTags(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test.rpm")
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
		require.NoError(t, err)
	}()

	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	version, err := rpm.Header.GetString(rpmutils.VERSION)
	require.NoError(t, err)
	require.Equal(t, "1.0.0-3", version)

	release, err := rpm.Header.GetString(rpmutils.RELEASE)
	require.NoError(t, err)
	require.Equal(t, "3", release)

	epoch, err := rpm.Header.Get(rpmutils.EPOCH)
	require.NoError(t, err)
	epochUint32, ok := epoch.([]uint32)
	require.Len(t, epochUint32, 1)
	require.True(t, ok)
	require.Equal(t, uint32(42), epochUint32[0])

	group, err := rpm.Header.GetString(rpmutils.GROUP)
	require.NoError(t, err)
	require.Equal(t, "default", group)

	summary, err := rpm.Header.GetString(rpmutils.SUMMARY)
	require.NoError(t, err)
	require.Equal(t, "first line", summary)

	description, err := rpm.Header.GetString(rpmutils.DESCRIPTION)
	require.NoError(t, err)
	require.Equal(t, info.Description, description)
}

func TestRPMVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0-1", meta.Version)
	require.Equal(t, "1", meta.Release)
}

func TestRPMVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0" //nolint:golint,goconst
	info.Release = "2"
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0-2", meta.Version)
	require.Equal(t, "2", meta.Release)
}

func TestRPMVersionWithPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()

	info.Version = "1.0.0"
	info.Prerelease = "rc1" // nolint:goconst
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0~rc1-1", meta.Version)
	require.Equal(t, "1", meta.Release)

	info.Version = "1.0.0~rc1"
	info.Prerelease = ""
	meta, err = buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0~rc1-1", meta.Version)
	require.Equal(t, "1", meta.Release)
}

func TestRPMVersionWithReleaseAndPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()

	info.Version = "1.0.0"
	info.Release = "2"
	info.Prerelease = "rc1"
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0~rc1-2", meta.Version)
	require.Equal(t, "2", meta.Release)

	info.Version = "1.0.0~rc1"
	info.Release = "3"
	info.Prerelease = ""
	meta, err = buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0~rc1-3", meta.Version)
	require.Equal(t, "3", meta.Release)
}

func TestRPMVersionWithVersionMetadata(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()

	info.Version = "1.0.0+meta"
	info.VersionMetadata = ""
	meta, err := buildRPMMeta(info)
	require.NoError(t, err)
	require.Equal(t, "1.0.0+meta-1", meta.Version)

	info.Version = "1.0.0"
	info.VersionMetadata = "meta"
	info.Release = "10"
	meta, err = buildRPMMeta(nfpm.WithDefaults(info))
	require.NoError(t, err)
	require.Equal(t, "1.0.0+meta-10", meta.Version)
}

func TestWithInvalidEpoch(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test.rpm")
	defer func() {
		_ = f.Close()
		err = os.Remove(f.Name())
		require.NoError(t, err)
	}()

	info := exampleInfo()
	info.Release = "3"
	info.Epoch = "-1"
	info.RPM = nfpm.RPM{
		Group: "default",
	}
	info.Description = "first line\nsecond line\nthird line"
	require.Error(t, Default.Package(info, f))
}

func TestRPMScripts(t *testing.T) {
	info := exampleInfo()
	f, err := os.CreateTemp(t.TempDir(), fmt.Sprintf("%s-%s-*.rpm", info.Name, info.Version))
	require.NoError(t, err)
	err = Default.Package(info, f)
	require.NoError(t, err)
	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0o600) //nolint:gosec
	require.NoError(t, err)
	defer func() {
		f.Close()
		file.Close()
		err = os.Remove(file.Name())
		require.NoError(t, err)
	}()
	rpm, err := rpmutils.ReadRpm(file)
	require.NoError(t, err)

	data, err := rpm.Header.GetString(rpmutils.PREIN)
	require.NoError(t, err)
	require.Equal(t, `#!/bin/bash

echo "Preinstall" > /dev/null
`, data, "Preinstall script does not match")

	data, err = rpm.Header.GetString(rpmutils.PREUN)
	require.NoError(t, err)
	require.Equal(t, `#!/bin/bash

echo "Preremove" > /dev/null
`, data, "Preremove script does not match")

	data, err = rpm.Header.GetString(rpmutils.POSTIN)
	require.NoError(t, err)
	require.Equal(t, `#!/bin/bash

echo "Postinstall" > /dev/null
`, data, "Postinstall script does not match")

	data, err = rpm.Header.GetString(rpmutils.POSTUN)
	require.NoError(t, err)
	require.Equal(t, `#!/bin/bash

echo "Postremove" > /dev/null
`, data, "Postremove script does not match")

	rpmPreTransTag := 1151
	data, err = rpm.Header.GetString(rpmPreTransTag)
	require.NoError(t, err)
	require.Equal(t, `#!/bin/bash

echo "Pretrans" > /dev/null
`, data, "Pretrans script does not match")

	rpmPostTransTag := 1152
	data, err = rpm.Header.GetString(rpmPostTransTag)
	require.NoError(t, err)
	require.Equal(t, `#!/bin/bash

echo "Posttrans" > /dev/null
`, data, "Posttrans script does not match")
}

func TestRPMFileDoesNotExist(t *testing.T) {
	info := exampleInfo()
	info.Contents = []*files.Content{
		{
			Source:      "../testdata/fake",
			Destination: "/usr/bin/fake",
		},
		{
			Source:      "../testdata/whatever.confzzz",
			Destination: "/etc/fake/fake.conf",
			Type:        files.TypeConfig,
		},
	}
	abs, err := filepath.Abs("../testdata/whatever.confzzz")
	require.NoError(t, err)
	err = Default.Package(info, io.Discard)
	require.EqualError(t, err, fmt.Sprintf("matching \"%s\": file does not exist", filepath.ToSlash(abs)))
}

func TestArches(t *testing.T) {
	for k := range archToRPM {
		t.Run(k, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = k
			info = setDefaults(info)
			require.Equal(t, archToRPM[k], info.Arch)
		})
	}

	t.Run("override", func(t *testing.T) {
		info := exampleInfo()
		info.RPM.Arch = "foo64"
		info = setDefaults(info)
		require.Equal(t, "foo64", info.Arch)
	})
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
		Maintainer:  "maintainer",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      buildConfigFile,
					Destination: packageConfigFile,
					Type:        files.TypeConfigNoReplace,
				},
			},
		},
	}

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	expectedConfigContent, err := os.ReadFile(buildConfigFile)
	require.NoError(t, err)

	packageConfigContent, err := extractFileFromRpm(rpmFileBuffer.Bytes(), packageConfigFile)
	require.NoError(t, err)

	require.Equal(t, expectedConfigContent, packageConfigContent)
}

func TestRPMConventionalFileName(t *testing.T) {
	info := &nfpm.Info{
		Name:       "testpkg",
		Arch:       "noarch",
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
			Expected: fmt.Sprintf("%s-1.2.3-1.%s.rpm", info.Name, info.Arch),
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
			Expected: fmt.Sprintf("%s-1.2.3~5-1.%s.rpm", info.Name, info.Arch),
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

		require.Equal(t, testCase.Expected, Default.ConventionalFileName(info))
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
	require.Equal(t, len(changelog), len(times))

	_titles, err := rpm.Header.Get(tagChangelogName)
	require.NoError(t, err)
	titles, ok := _titles.([]string)
	require.True(t, ok)
	require.Equal(t, len(changelog), len(titles))

	_notes, err := rpm.Header.Get(tagChangelogText)
	require.NoError(t, err)
	allNotes, ok := _notes.([]string)
	require.True(t, ok)
	require.Equal(t, len(changelog), len(allNotes))

	for i, entry := range changelog {
		timestamp := time.Unix(int64(times[i]), 0).UTC()
		title := titles[i]
		notes := strings.Split(allNotes[i], "\n")

		require.Equal(t, entry.Date, timestamp)
		require.True(t, strings.Contains(title, entry.Packager))
		require.True(t, strings.Contains(title, entry.Semver))
		require.Equal(t, len(entry.Changes), len(notes))

		for j, change := range entry.Changes {
			require.True(t, strings.Contains(notes[j], change.Note))
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
	require.Error(t, err)

	_, err = rpm.Header.Get(tagChangelogName)
	require.Error(t, err)

	_, err = rpm.Header.Get(tagChangelogText)
	require.Error(t, err)
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

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	packagedSymlinkHeader, err := extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), symlink)
	require.NoError(t, err)

	packagedSymlink, err := extractFileFromRpm(rpmFileBuffer.Bytes(), symlink)
	require.NoError(t, err)

	require.Equal(t, symlink, packagedSymlinkHeader.Filename())
	require.Equal(t, cpio.S_ISLNK, packagedSymlinkHeader.Mode())
	require.Equal(t, symlinkTarget, string(packagedSymlink))
}

func TestRPMSignature(t *testing.T) {
	info := exampleInfo()
	info.RPM.Signature.KeyFile = "../internal/sign/testdata/privkey.asc"
	info.RPM.Signature.KeyPassphrase = "hunter2"

	pubkeyFileContent, err := os.ReadFile("../internal/sign/testdata/pubkey.gpg")
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

func TestRPMSignatureCallback(t *testing.T) {
	info := exampleInfo()
	info.RPM.Signature.SignFn = func(r io.Reader) ([]byte, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		return sign.PGPSignerWithKeyID("../internal/sign/testdata/privkey.asc", "hunter2", nil)(data)
	}

	pubkeyFileContent, err := os.ReadFile("../internal/sign/testdata/pubkey.gpg")
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

func TestRPMGhostFiles(t *testing.T) {
	filename := "/usr/lib/casper.a"

	info := &nfpm.Info{
		Name:        "rpm-ghost",
		Arch:        "amd64",
		Description: "This RPM contains ghost files.",
		Version:     "1.0.0",
		Maintainer:  "maintainer",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Destination: filename,
					Type:        files.TypeRPMGhost,
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
	require.Equal(t, expected, actual)

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

	expectedContent, err := os.ReadFile("../testdata/{file}[")
	require.NoError(t, err)

	actualContent, err := extractFileFromRpm(rpmFileBuffer.Bytes(), "/test/{file}[")
	require.NoError(t, err)

	require.Equal(t, expectedContent, actualContent)
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
		},
		{
			Destination: "/etc/baz",
			Type:        files.TypeDir,
			FileInfo: &files.ContentFileInfo{
				Owner: "test",
				Mode:  0o700,
			},
		},
		{
			Destination: "/usr/lib/something/somethingelse",
			Type:        files.TypeDir,
		},
	}

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	require.Equal(t, []string{
		"/etc/bar",
		"/etc/bar/file",
		"/etc/baz",
		"/etc/foo/file",
		"/usr/lib/something/somethingelse",
	}, getTree(t, rpmFileBuffer.Bytes()))

	// the directory /etc/foo should not be implicitly created as that
	// implies ownership of /etc/foo which should always be implicit
	_, err = extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), "/etc/foo")
	require.Equal(t, err, os.ErrNotExist)

	// claiming explicit ownership of /etc/bar which already contains a file
	h, err := extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), "/etc/bar")
	require.NoError(t, err)
	require.NotEqual(t, h.Mode()&int(tagDirectory), 0)

	// creating an empty folder (which also implies ownership)
	h, err = extractFileHeaderFromRpm(rpmFileBuffer.Bytes(), "/etc/baz")
	require.NoError(t, err)
	require.Equal(t, h.Mode(), int(tagDirectory|0o700))
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

func TestIgnoreUnrelatedFiles(t *testing.T) {
	info := exampleInfo()
	info.Contents = files.Contents{
		{
			Source:      "../testdata/fake",
			Destination: "/some/file",
			Type:        files.TypeDebChangelog,
		},
	}

	var rpmFileBuffer bytes.Buffer
	err := Default.Package(info, &rpmFileBuffer)
	require.NoError(t, err)

	require.Len(t, getTree(t, rpmFileBuffer.Bytes()), 0)
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

		fileContents, err := io.ReadAll(pr)
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

func getTree(tb testing.TB, rpm []byte) []string {
	tb.Helper()

	rpmFile, err := rpmutils.ReadRpm(bytes.NewReader(rpm))
	require.NoError(tb, err)
	pr, err := rpmFile.PayloadReader()
	require.NoError(tb, err)

	var tree []string
	for {
		hdr, err := pr.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		require.NoError(tb, err)
		tree = append(tree, hdr.Filename())
	}

	return tree
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
