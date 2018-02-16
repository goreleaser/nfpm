package rpm

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
)

func TestRPM(t *testing.T) {
	var err = Default.Package(
		nfpm.WithDefaults(nfpm.Info{
			Name: "foo",
			Arch: "amd64",
			Depends: []string{
				"bash",
			},
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			License:     "MIT",
			Bindir:      "/usr/local/bin",
			Files: map[string]string{
				"../testdata/fake": "/usr/local/bin/fake",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.conf": "/etc/fake/fake.conf",
			},
		}),
		ioutil.Discard,
	)
	assert.NoError(t, err)
}

func TestNoFiles(t *testing.T) {
	var err = Default.Package(
		nfpm.WithDefaults(nfpm.Info{
			Name: "foo",
			Arch: "amd64",
			Depends: []string{
				"bash",
			},
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			License:     "MIT",
			Bindir:      "/usr/local/bin",
		}),
		ioutil.Discard,
	)
	assert.Error(t, err)
}

func TestRPMBuildNotInPath(t *testing.T) {
	assert.NoError(t, os.Setenv("PATH", ""))
	var err = Default.Package(
		nfpm.WithDefaults(nfpm.Info{}),
		ioutil.Discard,
	)
	assert.EqualError(t, err, `rpmbuild failed: exec: "rpmbuild": executable file not found in $PATH`)
}
