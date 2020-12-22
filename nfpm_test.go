package nfpm_test

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm"
	"github.com/goreleaser/nfpm/files"
)

func TestRegister(t *testing.T) {
	format := "TestRegister"
	pkgr := &fakePackager{}
	nfpm.RegisterPackager(format, pkgr)
	got, err := nfpm.Get(format)
	require.NoError(t, err)
	assert.Equal(t, pkgr, got)
}

func TestGet(t *testing.T) {
	format := "TestGet"
	got, err := nfpm.Get(format)
	require.Error(t, err)
	assert.EqualError(t, err, "no packager registered for the format "+format)
	assert.Nil(t, got)
	pkgr := &fakePackager{}
	nfpm.RegisterPackager(format, pkgr)
	got, err = nfpm.Get(format)
	require.NoError(t, err)
	assert.Equal(t, pkgr, got)
}

func TestDefaultsVersion(t *testing.T) {
	info := &nfpm.Info{
		Version: "v1.0.0",
	}
	info = nfpm.WithDefaults(info)
	assert.NotEmpty(t, info.Platform)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "", info.Release)
	assert.Equal(t, "", info.Prerelease)

	info = &nfpm.Info{
		Version: "v1.0.0-rc1",
	}
	info = nfpm.WithDefaults(info)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "", info.Release)
	assert.Equal(t, "rc1", info.Prerelease)

	info = &nfpm.Info{
		Version: "v1.0.0-beta1",
	}
	info = nfpm.WithDefaults(info)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "", info.Release)
	assert.Equal(t, "beta1", info.Prerelease)

	info = &nfpm.Info{
		Version:    "v1.0.0-1",
		Release:    "2",
		Prerelease: "beta1",
	}
	info = nfpm.WithDefaults(info)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "2", info.Release)
	assert.Equal(t, "beta1", info.Prerelease)

	info = &nfpm.Info{
		Version:    "v1.0.0-1+xdg2",
		Release:    "2",
		Prerelease: "beta1",
	}
	info = nfpm.WithDefaults(info)
	assert.Equal(t, "1.0.0", info.Version)
	assert.Equal(t, "2", info.Release)
	assert.Equal(t, "beta1", info.Prerelease)
	assert.Equal(t, "", info.Deb.VersionMetadata)
}

func TestDefaults(t *testing.T) {
	info := &nfpm.Info{
		Platform:    "darwin",
		Version:     "2.4.1",
		Description: "no description given",
	}
	got := nfpm.WithDefaults(info)
	assert.Equal(t, info, got)
}

func TestValidate(t *testing.T) {
	require.NoError(t, nfpm.Validate(&nfpm.Info{
		Name:    "as",
		Arch:    "asd",
		Version: "1.2.3",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "asd",
					Destination: "asd",
				},
			},
		},
	}))
	require.NoError(t, nfpm.Validate(&nfpm.Info{
		Name:    "as",
		Arch:    "asd",
		Version: "1.2.3",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "asd",
					Destination: "asd",
					Type:        "config",
				},
			},
		},
	}))
}

func TestValidateError(t *testing.T) {
	for err, info := range map[string]nfpm.Info{
		"package name must be provided": {},
		"package arch must be provided": {
			Name: "fo",
		},
		"package version must be provided": {
			Name: "as",
			Arch: "asd",
		},
	} {
		err := err
		info := info
		t.Run(err, func(t *testing.T) {
			require.EqualError(t, nfpm.Validate(&info), err)
		})
	}
}

func TestParseFile(t *testing.T) {
	nfpm.ClearPackagers()
	_, err := nfpm.ParseFile("./testdata/overrides.yaml")
	require.Error(t, err)
	nfpm.RegisterPackager("deb", &fakePackager{})
	nfpm.RegisterPackager("rpm", &fakePackager{})
	nfpm.RegisterPackager("apk", &fakePackager{})
	_, err = nfpm.ParseFile("./testdata/overrides.yaml")
	require.NoError(t, err)
	_, err = nfpm.ParseFile("./testdata/doesnotexist.yaml")
	require.Error(t, err)
	config, err := nfpm.ParseFile("./testdata/versionenv.yaml")
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("v%s", os.Getenv("GOROOT")), config.Version)
}

func TestParseEnhancedFile(t *testing.T) {
	config, err := nfpm.ParseFile("./testdata/contents.yaml")
	require.NoError(t, err)
	assert.Equal(t, config.Name, "contents foo")
	shouldFind := 5
	if len(config.Contents) != shouldFind {
		t.Errorf("should have had %d files but found %d", shouldFind, len(config.Contents))
		for idx, f := range config.Contents {
			fmt.Printf("%d => %+#v\n", idx, f)
		}
	}
}

func TestParseEnhancedNestedGlobFile(t *testing.T) {
	config, err := nfpm.ParseFile("./testdata/contents_glob.yaml")
	require.NoError(t, err)
	shouldFind := 3
	if len(config.Contents) != shouldFind {
		t.Errorf("should have had %d files but found %d", shouldFind, len(config.Contents))
		for idx, f := range config.Contents {
			fmt.Printf("%d => %+#v\n", idx, f)
		}
	}
}

func TestOptionsFromEnvironment(t *testing.T) {
	const (
		globalPass = "hunter2"
		debPass    = "password123"
		rpmPass    = "secret"
		apkPass    = "foobar"
		release    = "3"
		version    = "1.0.0"
	)

	t.Run("version", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("VERSION", version)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nversion: $VERSION"))
		require.NoError(t, err)
		assert.Equal(t, version, info.Version)
	})

	t.Run("release", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("RELEASE", release)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nrelease: $RELEASE"))
		require.NoError(t, err)
		assert.Equal(t, release, info.Release)
	})

	t.Run("global passphrase", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NFPM_PASSPHRASE", globalPass)
		info, err := nfpm.Parse(strings.NewReader("name: foo"))
		require.NoError(t, err)
		assert.Equal(t, globalPass, info.Deb.Signature.KeyPassphrase)
		assert.Equal(t, globalPass, info.RPM.Signature.KeyPassphrase)
		assert.Equal(t, globalPass, info.APK.Signature.KeyPassphrase)
	})

	t.Run("specific passphrases", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NFPM_PASSPHRASE", globalPass)
		os.Setenv("NFPM_DEB_PASSPHRASE", debPass)
		os.Setenv("NFPM_RPM_PASSPHRASE", rpmPass)
		os.Setenv("NFPM_APK_PASSPHRASE", apkPass)
		info, err := nfpm.Parse(strings.NewReader("name: foo"))
		require.NoError(t, err)
		assert.Equal(t, debPass, info.Deb.Signature.KeyPassphrase)
		assert.Equal(t, rpmPass, info.RPM.Signature.KeyPassphrase)
		assert.Equal(t, apkPass, info.APK.Signature.KeyPassphrase)
	})
}

func TestOverrides(t *testing.T) {
	file := "./testdata/overrides.yaml"
	config, err := nfpm.ParseFile(file)
	require.NoError(t, err)
	assert.Equal(t, "foo", config.Name)
	assert.Equal(t, "amd64", config.Arch)

	// deb overrides
	deb, err := config.Get("deb")
	require.NoError(t, err)
	assert.Contains(t, deb.Depends, "deb_depend")
	assert.NotContains(t, deb.Depends, "rpm_depend")
	for _, f := range deb.Contents {
		fmt.Printf("%+#v\n", f)
		assert.True(t, f.Packager != "rpm")
		assert.True(t, f.Packager != "apk")
		if f.Packager == "deb" {
			assert.Contains(t, f.Destination, "/deb")
		}
		if f.Packager == "" {
			assert.True(t, f.Destination == "/etc/foo/whatever.conf")
		}
	}
	assert.Equal(t, "amd64", deb.Arch)

	// rpm overrides
	rpm, err := config.Get("rpm")
	require.NoError(t, err)
	assert.Contains(t, rpm.Depends, "rpm_depend")
	assert.NotContains(t, rpm.Depends, "deb_depend")
	for _, f := range rpm.Contents {
		fmt.Printf("%+#v\n", f)
		assert.True(t, f.Packager != "deb")
		assert.True(t, f.Packager != "apk")
		if f.Packager == "rpm" {
			assert.Contains(t, f.Destination, "/rpm")
		}
		if f.Packager == "" {
			assert.True(t, f.Destination == "/etc/foo/whatever.conf")
		}
	}
	assert.Equal(t, "amd64", rpm.Arch)

	// no overrides
	info, err := config.Get("doesnotexist")
	require.NoError(t, err)
	assert.True(t, reflect.DeepEqual(&config.Info, info))
}

type fakePackager struct{}

func (*fakePackager) ConventionalFileName(info *nfpm.Info) string {
	return ""
}

func (*fakePackager) Package(info *nfpm.Info, w io.Writer) error {
	return nil
}
