package deb

import (
	"bytes"
	"flag"
	"io/ioutil"
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
	Files: map[string]string{
		"../testdata/fake": "/usr/local/bin/fake",
	},
	ConfigFiles: map[string]string{
		"../testdata/whatever.conf": "/etc/fake/fake.conf",
	},
})

func TestDeb(t *testing.T) {
	var err = Default.Package(info, ioutil.Discard)
	assert.NoError(t, err)
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info:          info,
		InstalledSize: 10,
	}))
	var golden = "testdata/control.golden"
	if *update {
		ioutil.WriteFile(golden, w.Bytes(), 0655)
	}
	bts, err := ioutil.ReadFile(golden)
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestDebFileDoesNotExist(t *testing.T) {
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
			Files: map[string]string{
				"../testdata/": "/usr/local/bin/fake",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.confzzz": "/etc/fake/fake.conf",
			},
		}),
		ioutil.Discard,
	)
	assert.Error(t, err)
}

func TestDebNoFiles(t *testing.T) {
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
		}),
		ioutil.Discard,
	)
	assert.NoError(t, err)
}

func TestDebNoInfo(t *testing.T) {
	var err = Default.Package(nfpm.WithDefaults(nfpm.Info{}), ioutil.Discard)
	assert.NoError(t, err)
}

func TestConffiles(t *testing.T) {
	out := conffiles(nfpm.Info{
		ConfigFiles: map[string]string{
			"fake": "/etc/fake",
		},
	})
	assert.Equal(t, "/etc/fake\n", string(out), "should have a trailing empty line")
}
