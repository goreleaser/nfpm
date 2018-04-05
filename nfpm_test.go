package nfpm

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	info := Info{
		Version: "2.4.1",
	}
	info = WithDefaults(info)
	assert.NotEmpty(t, info.Bindir)
	assert.NotEmpty(t, info.Platform)
	assert.Equal(t, "2.4.1", info.Version)
}

func TestDefaults(t *testing.T) {
	info := Info{
		Bindir:      "/usr/bin",
		Platform:    "darwin",
		Version:     "2.4.1",
		Description: "no description given",
	}
	got := WithDefaults(info)
	assert.Equal(t, info, got)
}

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(Info{
		Name:    "as",
		Arch:    "asd",
		Version: "1.2.3",
		Files: map[string]string{
			"asa": "asd",
		},
	}))
	require.NoError(t, Validate(Info{
		Name:    "as",
		Arch:    "asd",
		Version: "1.2.3",
		ConfigFiles: map[string]string{
			"asa": "asd",
		},
	}))
}

func TestValidateError(t *testing.T) {
	for err, info := range map[string]Info{
		"package name cannot be empty": Info{},
		"package arch must be provided": Info{
			Name: "fo",
		},
		"package version must be provided": Info{
			Name: "as",
			Arch: "asd",
		},
		"no files were provided": Info{
			Name:    "as",
			Arch:    "asd",
			Version: "1.2.3",
		},
	} {
		t.Run(err, func(t *testing.T) {
			require.EqualError(t, Validate(info), err)
		})
	}
}

type fakePackager struct{}

func (*fakePackager) Package(info Info, w io.Writer) error {
	return nil
}
