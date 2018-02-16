package nfpm

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegister(t *testing.T) {
	format := "TestRegister"
	pkgr := &fakePackager{}
	Register(format, pkgr)
	got, err := Get(format)
	assert.NoError(t, err)
	assert.Equal(t, pkgr, got)
}

func TestGet(t *testing.T) {
	format := "TestGet"
	got, err := Get(format)
	assert.Error(t, err)
	assert.EqualError(t, err, "no packager registered for the format "+format)
	assert.Nil(t, got)
	pkgr := &fakePackager{}
	Register(format, pkgr)
	got, err = Get(format)
	assert.NoError(t, err)
	assert.Equal(t, pkgr, got)
}

func TestDefaultsOnEmptyInfo(t *testing.T) {
	info := Info{}
	info = WithDefaults(info)
	assert.NotEmpty(t, info.Bindir)
	assert.NotEmpty(t, info.Platform)
}

func TestDefaults(t *testing.T) {
	info := Info{
		Bindir:   "/usr/bin",
		Platform: "darwin",
	}
	got := WithDefaults(info)
	assert.Equal(t, info, got)
}

type fakePackager struct{}

func (*fakePackager) Package(info Info, w io.Writer) error {
	return nil
}
