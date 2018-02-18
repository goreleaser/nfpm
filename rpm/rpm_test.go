package rpm

import (
	"bytes"
	"flag"
	"io/ioutil"
	"os"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
)

var update = flag.Bool("update", false, "update .golden files")

var info = nfpm.WithDefaults(nfpm.Info{
	Name: "foo",
	Arch: "amd64",
	Depends: []string{
		"bash",
	},
	Recommends: []string{
		"git",
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
})

func TestSpec(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeSpec(&w, info))
	var golden = "testdata/spec.golden"
	if *update {
		ioutil.WriteFile(golden, w.Bytes(), 0655)
	}
	bts, err := ioutil.ReadFile(golden)
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestRPM(t *testing.T) {
	var err = Default.Package(info, ioutil.Discard)
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
