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
	for golden, vs := range map[string]rpmbuildVersion{
		"testdata/spec_4.14.x.golden": {4, 14, 2},
		"testdata/spec_4.13.x.golden": {4, 13, 1},
		"testdata/spec_4.12.x.golden": {4, 12, 9},
	} {
		t.Run(golden, func(tt *testing.T) {
			var w bytes.Buffer
			assert.NoError(tt, writeSpec(&w, info, vs))
			if *update {
				ioutil.WriteFile(golden, w.Bytes(), 0655)
			}
			bts, err := ioutil.ReadFile(golden)
			assert.NoError(tt, err)
			assert.Equal(tt, string(bts), w.String())
		})
	}
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
	path := os.Getenv("PATH")
	defer os.Setenv("PATH", path)
	assert.NoError(t, os.Setenv("PATH", ""))
	var err = Default.Package(
		nfpm.WithDefaults(nfpm.Info{}),
		ioutil.Discard,
	)
	assert.EqualError(t, err, `rpmbuild not present in $PATH`)
}

func TestRpmBuildVersion(t *testing.T) {
	v, err := getRpmbuildVersion()
	assert.NoError(t, err)
	assert.Equal(t, 4, v.Major)
	assert.True(t, v.Minor >= 11)
	assert.True(t, v.Path >= 0)
}

func TestRPMFileDoesNotExist(t *testing.T) {
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
	assert.EqualError(t, err, "../testdata/whatever.confzzz: file does not exist")
}

func TestParseRpmbuildVersion(t *testing.T) {
	for _, version := range []string{
		"RPM-Version 4.14.1",
		"RPM version 4.14.1",
		"RPM vers~ao 4.14.1",
		"RPM versão 4.14.1",
		"RPM-Versionzz 4.14.1",
	} {
		t.Run(version, func(t *testing.T) {
			v, err := parseRPMbuildVersion(version)
			assert.NoError(t, err)
			assert.Equal(t, 4, v.Major)
			assert.Equal(t, 14, v.Minor)
			assert.Equal(t, 1, v.Path)
		})
	}
}

func TestParseRpmbuildVersionError(t *testing.T) {
	for _, version := range []string{
		"nooo foo bar 1.2.3",
		"RPM version 4.14.a",
		"RPM version 4.14",
	} {
		t.Run(version, func(t *testing.T) {
			_, err := parseRPMbuildVersion(version)
			assert.Error(t, err)
		})
	}
}
