package apk

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/goreleaser/nfpm"

	"github.com/stretchr/testify/assert"
)

func TestRunitWithNFPMInfo(t *testing.T) {
	workDir := path.Join("testdata", "workdir")
	tempDir, err := ioutil.TempDir(workDir, "test-run")
	if err != nil {
		log.Fatal(err)
	}
	skipVerifyInfo := isSkipVerifyInfo()
	defer func() {
		if !skipVerifyInfo {
			// cleanup temp files
			assert.Nil(t, os.RemoveAll(tempDir))
		}
	}()

	apkFileToCreate, err := os.Create(path.Join(tempDir, "apkToCreate.apk"))
	assert.NoError(t, err)
	if skipVerifyInfo {
		t.Log("apk at", tempDir)
	}

	err = runit(&nfpm.Info{
		Name: "foo",
		Overridables: nfpm.Overridables{
			Files: map[string]string{
				path.Join("testdata", "files", "control.golden"): "/testdata/files/control.golden",
			},
			EmptyFolders: []string{
				"/testdata/files/emptydir",
			},
		},
	}, path.Join("testdata", "keyfile", "id_rsa"), apkFileToCreate)

	assert.Nil(t, err)

	if !skipVerifyInfo {
		verifyFileSizeRange(t, apkFileToCreate, 1342, 1399)
	}
}

func isSkipVerifyInfo() bool {
	skipVerifyInfo, _ := strconv.ParseBool(os.Getenv("skipVerifyInfo"))
	return skipVerifyInfo
}

func verifyFileSizeRange(t *testing.T, fileToVerify *os.File, expectedSizeMin, expectedSizeMax int64) {
	fi, err := fileToVerify.Stat()
	assert.Nil(t, err)
	assert.True(t, (expectedSizeMin <= fi.Size()) && (fi.Size() <= expectedSizeMax),
		"bad value range: expectedSizeMin: %d, expectedSizeMax: %d, actual: %d, file: %s", expectedSizeMin, expectedSizeMax, fi.Size(), fileToVerify) // yuck
}

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
			Apk: nfpm.Apk{
				PrivateKey: "asdf",
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
	info.Apk = nfpm.Apk{
		PrivateKey: path.Join("testdata", "keyfile", "id_rsa"),
	}

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

	var bufSignature bytes.Buffer
	bufSignature.Write([]byte{3})

	var bufTarget bytes.Buffer

	assert.NoError(t, combineToApk(&bufTarget, &bufData, &bufControl, &bufSignature))

	assert.Equal(t, 3, bufTarget.Len())
}
