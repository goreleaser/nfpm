package rpm

import (
	"os"
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/assert"
)

func TestRPM(t *testing.T) {
	f, err := os.Create("foo.rpm")
	assert.NoError(t, err)
	err = Default.Package(
		nfpm.Info{
			Name:     "foo",
			Arch:     "amd64",
			Platform: "linux",
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
				"../testdata/fake": "/usr/local/bin/fake",
			},
			ConfigFiles: map[string]string{
				"../testdata/whatever.conf": "/etc/fake/fake.conf",
			},
		},
		f,
	)
	assert.NoError(t, err)
}
