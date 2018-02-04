package rpm

import (
	"io/ioutil"
	"testing"

	"github.com/goreleaser/packager"
	"github.com/tj/assert"
)

func TestRPM(t *testing.T) {
	var err = Default.Package(
		packager.Info{
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
			Files: map[string]string{
				"./testdata/fake": "/usr/local/bin/fake",
			},
			ConfigFiles: map[string]string{
				"./testdata/whatever.conf": "/etc/fake/fake.conf",
			},
		},
		ioutil.Discard,
	)
	assert.NoError(t, err)
}
