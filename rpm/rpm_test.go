package rpm

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/sassoftware/go-rpmutils"
	"github.com/stretchr/testify/assert"

	"github.com/goreleaser/nfpm"
)

const (
	tagRelease = 0x03ea // 1002
	tagGroup   = 0x03f8 // 1016
	tagPrein   = 0x03ff // 1023
	tagPostin  = 0x0400 // 1024
	tagPreun   = 0x0401 // 1025
	tagPostun  = 0x0402 // 1026
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
		Bindir:      "/usr/local/bin",
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
	release, err := rpm.Header.GetString(tagRelease)
	assert.NoError(t, err)
	assert.Equal(t, "1", release)
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
	info.RPM = nfpm.RPM{
		Group: "default",
	}
	assert.NoError(t, Default.Package(info, f))

	file, err := os.OpenFile(f.Name(), os.O_RDONLY, 0600) //nolint:gosec
	assert.NoError(t, err)

	rpm, err := rpmutils.ReadRpm(file)
	assert.NoError(t, err)

	release, err := rpm.Header.GetString(tagRelease)
	assert.NoError(t, err)
	assert.Equal(t, "3", release)

	group, err := rpm.Header.GetString(tagGroup)
	assert.NoError(t, err)
	assert.Equal(t, "default", group)
}

func TestRPMVersionWithDash(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0-beta"
	var err = Default.Package(info, ioutil.Discard)
	assert.NoError(t, err)
}

func TestRPMVersion(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "1", meta.Release)
}

func TestRPMVersionWithRelease(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0"
	info.Release = "2"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "2", meta.Release)
}

func TestRPMVersionWithPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()
	info.Version = "1.0.0"
	info.Prerelease = "rc1"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "0.1.rc1", meta.Release)
}

func TestRPMVersionWithReleaseAndPrerelease(t *testing.T) {
	// https://fedoraproject.org/wiki/Package_Versioning_Examples#Complex_versioning_examples
	info := exampleInfo()
	info.Version = "1.0.0"
	info.Release = "0.2"
	info.Prerelease = "rc1"
	meta, err := buildRPMMeta(info)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", meta.Version)
	assert.Equal(t, "0.2.rc1", meta.Release)
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

func TestRPMNoFiles(t *testing.T) {
	info := exampleInfo()
	info.Files = map[string]string{}
	info.ConfigFiles = map[string]string{}
	var err = Default.Package(info, ioutil.Discard)
	// TODO: better deal with this error
	assert.Error(t, err)
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
