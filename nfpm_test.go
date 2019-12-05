package nfpm

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
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
	info := &Info{
		Version: "2.4.1",
	}
	info = WithDefaults(info)
	assert.NotEmpty(t, info.Bindir)
	assert.NotEmpty(t, info.Platform)
	assert.Equal(t, "2.4.1", info.Version)
}

func TestDefaults(t *testing.T) {
	info := &Info{
		Bindir:      "/usr/bin",
		Platform:    "darwin",
		Version:     "2.4.1",
		Description: "no description given",
	}
	got := WithDefaults(info)
	assert.Equal(t, info, got)
}

func TestValidate(t *testing.T) {
	require.NoError(t, Validate(&Info{
		Name:    "as",
		Arch:    "asd",
		Version: "1.2.3",
		Overridables: Overridables{
			Files: map[string]string{
				"asa": "asd",
			},
		},
	}))
	require.NoError(t, Validate(&Info{
		Name:    "as",
		Arch:    "asd",
		Version: "1.2.3",
		Overridables: Overridables{
			ConfigFiles: map[string]string{
				"asa": "asd",
			},
		},
	}))
}

func TestValidateError(t *testing.T) {
	for err, info := range map[string]Info{
		"package name cannot be empty": {},
		"package arch must be provided": {
			Name: "fo",
		},
		"package version must be provided": {
			Name: "as",
			Arch: "asd",
		},
		"no files were provided": {
			Name:    "as",
			Arch:    "asd",
			Version: "1.2.3",
		},
	} {
		err := err
		info := info
		t.Run(err, func(t *testing.T) {
			require.EqualError(t, Validate(&info), err)
		})
	}
}

func TestParseFile(t *testing.T) {
	packagers = map[string]Packager{}
	_, err := ParseFile("./testdata/overrides.yaml")
	assert.Error(t, err)
	Register("deb", &fakePackager{})
	Register("rpm", &fakePackager{})
	_, err = ParseFile("./testdata/overrides.yaml")
	assert.NoError(t, err)
	_, err = ParseFile("./testdata/doesnotexist.yaml")
	assert.Error(t, err)
	config, err := ParseFile("./testdata/versionenv.yaml")
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("v%s", os.Getenv("GOROOT")), config.Version)
}

func TestVersionParse(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`{ version: v1.0.0 }`)
	config, err := Parse(&buf)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "", config.Release)
	assert.Equal(t, "", config.Prerelease)

	buf.Reset()
	buf.WriteString(`{ version: v1.0.0-rc1 }`)
	config, err = Parse(&buf)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "", config.Release)
	assert.Equal(t, "rc1", config.Prerelease)

	buf.Reset()
	buf.WriteString(`{ version: v1.0.0-1 }`)
	config, err = Parse(&buf)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "1", config.Release)
	assert.Equal(t, "", config.Prerelease)

	buf.Reset()
	buf.WriteString(`{ version: v1.0.0-1, release: "2", prerelease: "beta1" }`)
	config, err = Parse(&buf)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "2", config.Release)
	assert.Equal(t, "beta1", config.Prerelease)

	buf.Reset()
	buf.WriteString(`{ version: v1.0.0-rc1, release: "2", prerelease: "beta1" }`)
	config, err = Parse(&buf)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", config.Version)
	assert.Equal(t, "2", config.Release)
	assert.Equal(t, "beta1", config.Prerelease)
}

func TestOverrides(t *testing.T) {
	file := "./testdata/overrides.yaml"
	config, err := ParseFile(file)
	assert.NoError(t, err)
	assert.Equal(t, "foo", config.Name)
	assert.Equal(t, "amd64", config.Arch)

	// deb overrides
	deb, err := config.Get("deb")
	assert.NoError(t, err)
	assert.Contains(t, deb.Depends, "deb_depend")
	assert.NotContains(t, deb.Depends, "rpm_depend")
	assert.Contains(t, deb.ConfigFiles, "deb.conf")
	assert.NotContains(t, deb.ConfigFiles, "rpm.conf")
	assert.Contains(t, deb.ConfigFiles, "whatever.conf")
	assert.Equal(t, "amd64", deb.Arch)

	// rpm overrides
	rpm, err := config.Get("rpm")
	assert.NoError(t, err)
	assert.Contains(t, rpm.Depends, "rpm_depend")
	assert.NotContains(t, rpm.Depends, "deb_depend")
	assert.Contains(t, rpm.ConfigFiles, "rpm.conf")
	assert.NotContains(t, rpm.ConfigFiles, "deb.conf")
	assert.Contains(t, rpm.ConfigFiles, "whatever.conf")
	assert.Equal(t, "amd64", rpm.Arch)

	// no overrides
	info, err := config.Get("doesnotexist")
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(&config.Info, info))
}

type fakePackager struct{}

func (*fakePackager) Package(info *Info, w io.Writer) error {
	return nil
}
