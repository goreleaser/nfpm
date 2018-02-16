package deb

import (
	"io/ioutil"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
)

func TestDeb(t *testing.T) {
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
