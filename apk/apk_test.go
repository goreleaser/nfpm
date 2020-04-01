package apk

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
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

	verifyFileSize(t, apkFileToCreate, 1384)

	verifyFileSize(t, path.Join(tempDir, "apk_control.tgz"), 302)
	verifyFileSize(t, path.Join(tempDir, "apk_data.tgz"), 416)
	verifyFileSize(t, path.Join(tempDir, "apk_signatures.tgz"), 666)
}

func verifyFileSize(t *testing.T, fileToVerify string, expectedSize int64) {
	fi, err := os.Stat(fileToVerify)
	assert.Nil(t, err)
	assert.Equal(t, expectedSize, fi.Size())
}