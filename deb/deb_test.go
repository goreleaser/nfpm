package deb

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
)

var update = flag.Bool("update", false, "update .golden files")

func exampleInfo() nfpm.Info {
	return nfpm.WithDefaults(nfpm.Info{
		Name: "foo",
		Arch: "amd64",
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
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "v1.0.0",
		Section:     "default",
		Homepage:    "http://carlosbecker.com",
		Vendor:      "nope",
		Files: map[string]string{
			"../testdata/fake":          "/usr/local/bin/fake",
			"../testdata/whatever.conf": "/usr/share/doc/fake/fake.txt",
		},
		ConfigFiles: map[string]string{
			"../testdata/whatever.conf": "/etc/fake/fake.conf",
		},
	})
}

func TestDeb(t *testing.T) {
	for _, arch := range []string{"386", "amd64"} {
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch
			var err = Default.Package(info, ioutil.Discard)
			assert.NoError(t, err)
		})
	}
}

func TestControl(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info:          exampleInfo(),
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

func TestNoJoinsControl(t *testing.T) {
	var w bytes.Buffer
	assert.NoError(t, writeControl(&w, controlData{
		Info: nfpm.WithDefaults(nfpm.Info{
			Name:        "foo",
			Arch:        "amd64",
			Depends:     []string{},
			Recommends:  []string{},
			Suggests:    []string{},
			Replaces:    []string{},
			Provides:    []string{},
			Conflicts:   []string{},
			Description: "Foo does things",
			Priority:    "extra",
			Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
			Version:     "v1.0.0",
			Section:     "default",
			Homepage:    "http://carlosbecker.com",
			Vendor:      "nope",
			Files:       map[string]string{},
			ConfigFiles: map[string]string{},
		}),
		InstalledSize: 10,
	}))
	var golden = "testdata/control2.golden"
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

func TestPathsToCreate(t *testing.T) {
	for path, parts := range map[string][]string{
		"/usr/share/doc/whatever/foo.md": []string{"usr", "usr/share", "usr/share/doc", "usr/share/doc/whatever"},
		"/var/moises":                    []string{"var"},
		"/":                              []string{},
	} {
		t.Run(fmt.Sprintf("path: '%s'", path), func(t *testing.T) {
			assert.Equal(t, parts, pathsToCreate(path))
		})
	}
}
