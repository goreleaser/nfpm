package deb

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/caarlos0/pkg"
	"github.com/tj/assert"
)

func TestDeb(t *testing.T) {
	var files = []pkg.File{
		{Src: "./testdata/fake", Dst: "/usr/local/bin/fake"},
		{Src: "./testdata/whatever.conf", Dst: "/etc/fake/fake.conf"},
	}
	var err = New(
		context.Background(),
		pkg.Info{
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
		},
		files,
		ioutil.Discard,
	)
	assert.NoError(t, err)
}
