package deb

import (
	"context"
	"testing"

	"github.com/caarlos0/pkg"
	"github.com/tj/assert"
)

func TestDeb(t *testing.T) {
	deb, err := New(context.Background(), pkg.Info{
		Name: "foo",
		Arch: "amd64",
		Depends: []string{
			"git",
		},
		Description: "Foo does things",
		Priority:    "extra",
		Maintainer:  "Carlos A Becker <pkg@carlosbecker.com>",
		Version:     "1.0.0",
		Section:     "default",
		Filename:    "/tmp/foo_1.0.0-0",
	})
	assert.NoError(t, err)
	assert.NoError(t, deb.Add("./testdata/fake", "/usr/local/bin/testdata"))
	assert.NoError(t, deb.Close())
}
