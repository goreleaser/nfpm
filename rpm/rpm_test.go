package rpm

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nolint: gochecknoglobals
var update = flag.Bool("update", false, "update .golden files")

func exampleInfo() nfpm.Info {
	return nfpm.WithDefaults(nfpm.Info{
		Name:        "foo",
		Arch:        "amd64",
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "1.0.0",
		Section:     "default",
		Homepage:    "http://carlosbecker.com",
		Vendor:      "nope",
		License:     "MIT",
		Bindir:      "/usr/local/bin",
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
				"../testdata/fake": "/usr/local/bin/fake",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.conf": "/etc/fake/fake.conf",
			},
			EmptyFolders: []string{
				"/var/log/whatever",
				"/usr/share/whatever",
			},
			Scripts: nfpm.Scripts{
				PreInstall:  "../testdata/scripts/preinstall.sh",
				PostInstall: "../testdata/scripts/postinstall.sh",
				PreRemove:   "../testdata/scripts/preremove.sh",
				PostRemove:  "../testdata/scripts/postremove.sh",
			},
		},
	})
}

func TestSpec(t *testing.T) {
	for golden, vs := range map[string]rpmbuildVersion{
		"testdata/spec_4.14.x.golden": {Major: 4, Minor: 14, Patch: 2},
		"testdata/spec_4.13.x.golden": {Major: 4, Minor: 13, Patch: 1},
		"testdata/spec_4.12.x.golden": {Major: 4, Minor: 12, Patch: 9},
	} {
		vs := vs
		golden := golden
		t.Run(golden, func(tt *testing.T) {
			var w bytes.Buffer
			assert.NoError(tt, writeSpec(&w, exampleInfo(), vs))
			if *update {
				require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0655))
			}
			bts, err := ioutil.ReadFile(golden) //nolint:gosec
			assert.NoError(tt, err)
			assert.Equal(tt, string(bts), w.String())
		})
	}
}

func TestRPM(t *testing.T) {
	var err = Default.Package(exampleInfo(), ioutil.Discard)
	assert.NoError(t, err)
}

func TestWithRPMTags(t *testing.T) {
	var info = exampleInfo()
	info.RPM = nfpm.RPM{
		Group:   "default",
		Prefix:  "/usr",
		Release: "3",
	}
	var err = Default.Package(info, ioutil.Discard)
	assert.NoError(t, err)
}

func TestRPMTagsSpec(t *testing.T) {
	var info = exampleInfo()
	info.RPM = nfpm.RPM{
		Group:   "default",
		Prefix:  "/usr",
		Release: "5",
	}

	vs := rpmbuildVersion{Major: 4, Minor: 15, Patch: 2}
	golden := "testdata/spec_4.15.x.golden"

	var w bytes.Buffer
	assert.NoError(t, writeSpec(&w, info, vs))
	if *update {
		require.NoError(t, ioutil.WriteFile(golden, w.Bytes(), 0655))
	}
	bts, err := ioutil.ReadFile(golden)
	assert.NoError(t, err)
	assert.Equal(t, string(bts), w.String())
}

func TestRPMVersionWithDash(t *testing.T) {
	info := exampleInfo()
	info.Version = "1.0.0-beta"
	var err = Default.Package(info, ioutil.Discard)
	assert.NoError(t, err)
}

func TestRPMScripts(t *testing.T) {
	info := exampleInfo()
	scripts, err := readScripts(info)
	assert.NoError(t, err)
	for actual, src := range map[string]string{
		scripts.Pre:    info.Scripts.PreInstall,
		scripts.Post:   info.Scripts.PostInstall,
		scripts.Preun:  info.Scripts.PreRemove,
		scripts.Postun: info.Scripts.PostRemove,
	} {
		data, err := ioutil.ReadFile(src)         //nolint:gosec
		fmt.Printf("%s %s %s", actual, src, data) //nolint.govet
		assert.NoError(t, err)
	}
}

func TestRPMNoFiles(t *testing.T) {
	info := exampleInfo()
	info.Files = map[string]string{}
	info.ConfigFiles = map[string]string{}
	var err = Default.Package(info, ioutil.Discard)
	// TODO: better deal with this error
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

func TestRPMBuildVersion(t *testing.T) {
	v, err := getRpmbuildVersion()
	assert.NoError(t, err)
	assert.Equal(t, 4, v.Major)
	assert.True(t, v.Minor >= 11)
	assert.True(t, v.Patch >= 0)
}

func TestRPMFileDoesNotExist(t *testing.T) {
	info := exampleInfo()
	info.Files = map[string]string{
		"../testdata/": "/usr/local/bin/fake",
	}
	info.ConfigFiles = map[string]string{
		"../testdata/whatever.confzzz": "/etc/fake/fake.conf",
	}
	var err = Default.Package(info, ioutil.Discard)
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
		version := version
		t.Run(version, func(t *testing.T) {
			v, err := parseRPMbuildVersion(version)
			assert.NoError(t, err)
			assert.Equal(t, 4, v.Major)
			assert.Equal(t, 14, v.Minor)
			assert.Equal(t, 1, v.Patch)
		})
	}
}

func TestParseRpmbuildVersionError(t *testing.T) {
	for _, version := range []string{
		"nooo foo bar 1.2.3",
		"RPM version 4.14.a",
		"RPM version 4.14",
	} {
		version := version
		t.Run(version, func(t *testing.T) {
			_, err := parseRPMbuildVersion(version)
			assert.Error(t, err)
		})
	}
}

func TestRPMMultiArch(t *testing.T) {
	info := exampleInfo()

	for k := range archToRPM {
		info.Arch = k
		info = ensureValidArch(info)
		assert.Equal(t, archToRPM[k], info.Arch)
	}
}
