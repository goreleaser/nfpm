package apk

import (
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/goreleaser/nfpm"

	"github.com/stretchr/testify/assert"
)

func TestRunit(t *testing.T) {
	workDir := path.Join("testdata", "workdir")
	tempDir, err := ioutil.TempDir(workDir, "test-run")
	if err != nil {
		log.Fatal(err)
	}
	skipVerify, err := strconv.ParseBool(os.Getenv("skipVerify"))
	// ignore env var parse error
	defer func() {
		if !skipVerify {
			// cleanup temp files
			assert.Nil(t, os.RemoveAll(tempDir))
		}
	}()

	apkFileToCreate := path.Join(tempDir, "apkToCreate.apk")
	t.Log("apk at", tempDir)

	err = runit(
		path.Join("testdata", "files"),
		path.Join("testdata", "keyfile", "id_rsa"),
		tempDir,
		apkFileToCreate)

	assert.Nil(t, err)

	if !skipVerify {
		verifyFileSize(t, apkFileToCreate, 1383, 1342, 1379)

		verifyFileSize(t, path.Join(tempDir, "apk_control.tgz"), 304, 300, 305)
		verifyFileSize(t, path.Join(tempDir, "apk_data.tgz"), 414, 373, 407)
		verifyFileSize(t, path.Join(tempDir, "apk_signatures.tgz"), 665, 665, 667)
	}
}

func verifyFileSize(t *testing.T, fileToVerify string, expectedSize, expectedSizeCiMin, expectedSizeCiMax int64) {
	fi, err := os.Stat(fileToVerify)
	assert.Nil(t, err)
	ciEnv := os.Getenv("CI")
	if ciEnv != "" {
		assert.True(t, (expectedSizeCiMin <= fi.Size()) && (fi.Size() <= expectedSizeCiMax),
			"bad value range: expectedSizeCiMin: %d, expectedSizeCiMax: %d, actual: %d, file: %s", expectedSizeCiMin, expectedSizeCiMax, fi.Size(), fileToVerify) // yuck
	} else {
		assert.Equal(t, expectedSize, fi.Size(), "bad file size, file: %s", fileToVerify)
	}
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
	var err = Default.Package(info, ioutil.Discard)
	assert.NoError(t, err)
	assert.Equal(t, expectedArch, info.Arch)
}
