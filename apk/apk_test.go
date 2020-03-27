package apk

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"os"
	"path"
	"testing"
)

func TestRunit(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	fmt.Println(cwd)

	testdata := path.Join(cwd, "testdata")

	workDir := path.Join(testdata, "workdir")
	tempDir, err := ioutil.TempDir(workDir, "test-run")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		assert.Nil(t, os.RemoveAll(tempDir))
	}()

	apkFileToCreate := path.Join(tempDir, "apkToCreate.apk")

	err = runit(
		path.Join(testdata, "deb"),
		path.Join(testdata, "keyfile", "id_rsa"),
		tempDir,
		apkFileToCreate)

	assert.Nil(t, err)

	verifyNonEmptyFile(err, apkFileToCreate, t)

	verifyNonEmptyFile(err, path.Join(tempDir, "apk_control.tgz"), t)
	verifyNonEmptyFile(err, path.Join(tempDir, "apk_data.tgz"), t)
	verifyNonEmptyFile(err, path.Join(tempDir, "apk_signatures.tgz"), t)
}

func verifyNonEmptyFile(err error, fileToCreate string, t *testing.T) {
	fi, err := os.Stat(fileToCreate)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, fi.Size())
}
