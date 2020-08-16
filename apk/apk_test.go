package apk

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm"

	"github.com/stretchr/testify/assert"
)

// nolint: gochecknoglobals
var updateApk = flag.Bool("update-apk", false, "update apk .golden files")

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "v1.0.0",
		Release:     "r1",
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

func TestArchToAlpine(t *testing.T) {
	verifyArch(t, "", "")
	verifyArch(t, "abc", "abc")
	verifyArch(t, "386", "x86")
	verifyArch(t, "amd64", "x86_64")
	verifyArch(t, "arm", "armhf")
	verifyArch(t, "arm6", "armhf")
	verifyArch(t, "arm7", "armhf")
	verifyArch(t, "arm64", "aarch64")
}

func verifyArch(t *testing.T, nfpmArch, expectedArch string) {
	info := exampleInfo()
	info.Arch = nfpmArch

	assert.NoError(t, Default.Package(info, ioutil.Discard))
	assert.Equal(t, expectedArch, info.Arch)
}

func TestCreateBuilderData(t *testing.T) {
	info := exampleInfo()
	size := int64(0)
	builderData := createBuilderData(info, &size)

	// tw := tar.NewWriter(ioutil.Discard)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	assert.NoError(t, builderData(tw))

	assert.Equal(t, 1043464, buf.Len())
}

func TestCombineToApk(t *testing.T) {
	var bufData bytes.Buffer
	bufData.Write([]byte{1})

	var bufControl bytes.Buffer
	bufControl.Write([]byte{2})

	var bufTarget bytes.Buffer

	assert.NoError(t, combineToApk(&bufTarget, &bufData, &bufControl))
	assert.Equal(t, 2, bufTarget.Len())
}

func TestPathsToCreate(t *testing.T) {
	for pathToTest, parts := range map[string][]string{
		"/usr/share/doc/whatever/foo.md": {"usr", "usr/share", "usr/share/doc", "usr/share/doc/whatever"},
		"/var/moises":                    {"var"},
		"/":                              []string(nil),
	} {
		parts := parts
		pathToTest := pathToTest
		t.Run(fmt.Sprintf("pathToTest: '%s'", pathToTest), func(t *testing.T) {
			assert.Equal(t, parts, pathsToCreate(pathToTest))
		})
	}
}

func TestDefaultWithArch(t *testing.T) {
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

func TestNoInfo(t *testing.T) {
	var err = Default.Package(nfpm.WithDefaults(&nfpm.Info{}), ioutil.Discard)
	assert.NoError(t, err)
}

func TestFileDoesNotExist(t *testing.T) {
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

func TestNoFiles(t *testing.T) {
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

func TestCreateBuilderControl(t *testing.T) {
	info := exampleInfo()
	size := int64(12345)
	digest := sha256.New()
	dataDigest := digest.Sum(nil)
	builderControl := createBuilderControl(info, size, dataDigest)

	var controlTgz bytes.Buffer
	tw := tar.NewWriter(&controlTgz)
	assert.NoError(t, builderControl(tw))

	assert.Equal(t, 798, controlTgz.Len())

	stringControlTgz := controlTgz.String()
	assert.True(t, strings.HasPrefix(stringControlTgz, ".PKGINFO"))
	assert.Contains(t, stringControlTgz, "pkgname = "+info.Name)
	assert.Contains(t, stringControlTgz, "pkgver = "+info.Version+"-"+info.Release)
	assert.Contains(t, stringControlTgz, "pkgdesc = "+info.Description)
	assert.Contains(t, stringControlTgz, "url = "+info.Homepage)
	assert.Contains(t, stringControlTgz, "maintainer = "+info.Maintainer)
	assert.Contains(t, stringControlTgz, "replaces = "+info.Replaces[0])
	assert.Contains(t, stringControlTgz, "provides = "+info.Provides[0])
	assert.Contains(t, stringControlTgz, "depend = "+info.Depends[0])
	assert.Contains(t, stringControlTgz, "arch = "+info.Arch) // conversion using archToAlpine[info.Arch]) would occur in Package() method
	assert.Contains(t, stringControlTgz, "size = "+strconv.Itoa(int(size)))
	assert.Contains(t, stringControlTgz, "datahash = "+hex.EncodeToString(dataDigest))
}

func TestCreateBuilderControlScripts(t *testing.T) {
	info := exampleInfo()
	info.Scripts = nfpm.Scripts{
		PreInstall:  "../testdata/scripts/preinstall.sh",
		PostInstall: "../testdata/scripts/postinstall.sh",
		PreRemove:   "../testdata/scripts/preremove.sh",
		PostRemove:  "../testdata/scripts/postremove.sh",
	}

	size := int64(12345)
	digest := sha256.New()
	dataDigest := digest.Sum(nil)
	builderControl := createBuilderControl(info, size, dataDigest)

	var controlTgz bytes.Buffer
	tw := tar.NewWriter(&controlTgz)
	assert.NoError(t, builderControl(tw))

	stringControlTgz := controlTgz.String()
	assert.Contains(t, stringControlTgz, ".pre-install")
	assert.Contains(t, stringControlTgz, ".post-install")
	assert.Contains(t, stringControlTgz, ".pre-deinstall")
	assert.Contains(t, stringControlTgz, ".post-deinstall")
	assert.Contains(t, stringControlTgz, "datahash = "+hex.EncodeToString(dataDigest))
}

func TestControl(t *testing.T) {
	digest := sha256.New()
	dataDigest := digest.Sum(nil)

	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(),
		InstalledSize: 10,
		Datahash:      hex.EncodeToString(dataDigest),
	}))
	var golden = "testdata/control.golden"
	if *updateApk {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0655)) // nolint: gosec
	}
	bts, err := ioutil.ReadFile(golden) //nolint:gosec
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}
