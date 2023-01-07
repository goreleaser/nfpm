package nfpm_test

import (
	"fmt"
	"io"
	"net/mail"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	format := "TestRegister"
	pkgr := &fakePackager{}
	nfpm.RegisterPackager(format, pkgr)
	got, err := nfpm.Get(format)
	require.NoError(t, err)
	require.Equal(t, pkgr, got)
}

func TestGet(t *testing.T) {
	format := "TestGet"
	got, err := nfpm.Get(format)
	require.Error(t, err)
	require.EqualError(t, err, "no packager registered for the format "+format)
	require.Nil(t, got)
	pkgr := &fakePackager{}
	nfpm.RegisterPackager(format, pkgr)
	got, err = nfpm.Get(format)
	require.NoError(t, err)
	require.Equal(t, pkgr, got)
}

func TestDefaultsVersion(t *testing.T) {
	info := &nfpm.Info{
		Version:       "v1.0.0",
		VersionSchema: "semver",
	}
	info = nfpm.WithDefaults(info)
	require.NotEmpty(t, info.Platform)
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "", info.Release)
	require.Equal(t, "", info.Prerelease)

	info = &nfpm.Info{
		Version: "v1.0.0-rc1",
	}
	info = nfpm.WithDefaults(info)
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "", info.Release)
	require.Equal(t, "rc1", info.Prerelease)

	info = &nfpm.Info{
		Version: "v1.0.0-beta1",
	}
	info = nfpm.WithDefaults(info)
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "", info.Release)
	require.Equal(t, "beta1", info.Prerelease)

	info = &nfpm.Info{
		Version:    "v1.0.0-1",
		Release:    "2",
		Prerelease: "beta1",
	}
	info = nfpm.WithDefaults(info)
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "2", info.Release)
	require.Equal(t, "beta1", info.Prerelease)

	info = &nfpm.Info{
		Version:    "v1.0.0-1+xdg2",
		Release:    "2",
		Prerelease: "beta1",
	}
	info = nfpm.WithDefaults(info)
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "2", info.Release)
	require.Equal(t, "beta1", info.Prerelease)

	info = &nfpm.Info{
		Version:       "this.is.my.version",
		VersionSchema: "none",
		Release:       "2",
		Prerelease:    "beta1",
	}
	info = nfpm.WithDefaults(info)
	require.Equal(t, "this.is.my.version", info.Version)
	require.Equal(t, "2", info.Release)
	require.Equal(t, "beta1", info.Prerelease)
}

func TestDefaults(t *testing.T) {
	info := &nfpm.Info{
		Platform:    "darwin",
		Version:     "2.4.1",
		Description: "no description given",
	}
	got := nfpm.WithDefaults(info)
	require.Equal(t, info, got)
}

func TestValidate(t *testing.T) {
	t.Run("dirs", func(t *testing.T) {
		info := nfpm.Info{
			Name:    "as",
			Arch:    "asd",
			Version: "1.2.3",
			Overridables: nfpm.Overridables{
				Contents: []*files.Content{
					{
						Destination: "/usr/share/test",
						Type:        "dir",
					},
					{
						Source:      "./testdata/contents.yaml",
						Destination: "asd",
					},
				},
			},
		}
		require.NoError(t, nfpm.Validate(&info))
		require.Len(t, info.Overridables.Contents, 2)
		dir := info.Overridables.Contents[0]
		require.Equal(t, "/usr/share/test", dir.Destination)
		require.Equal(t, "dir", dir.Type)
		require.Equal(t, "-rwxr-xr-x", dir.FileInfo.Mode.String())
		require.Equal(t, "root", dir.FileInfo.Owner)
		require.Equal(t, "root", dir.FileInfo.Group)
	})

	t.Run("config", func(t *testing.T) {
		require.NoError(t, nfpm.Validate(&nfpm.Info{
			Name:    "as",
			Arch:    "asd",
			Version: "1.2.3",
			Overridables: nfpm.Overridables{
				Contents: []*files.Content{
					{
						Source:      "./testdata/contents.yaml",
						Destination: "asd",
						Type:        "config",
					},
				},
			},
		}))
	})
}

func TestValidateError(t *testing.T) {
	for err, info := range map[string]*nfpm.Info{
		"package name must be provided": {},
		"package arch must be provided": {
			Name: "fo",
		},
		"package version must be provided": {
			Name: "as",
			Arch: "asd",
		},
	} {
		func(inf *nfpm.Info, e string) {
			t.Run(e, func(t *testing.T) {
				require.EqualError(t, nfpm.Validate(inf), e)
			})
		}(info, err)
	}
}

func parseAndValidate(filename string) (nfpm.Config, error) {
	config, err := nfpm.ParseFile(filename)
	if err == nil {
		err = config.Validate()
	}

	return config, err
}

func TestParseFile(t *testing.T) {
	nfpm.ClearPackagers()
	_, err := parseAndValidate("./testdata/overrides.yaml")
	require.Error(t, err)
	nfpm.RegisterPackager("deb", &fakePackager{})
	nfpm.RegisterPackager("rpm", &fakePackager{})
	nfpm.RegisterPackager("apk", &fakePackager{})
	_, err = parseAndValidate("./testdata/overrides.yaml")
	require.NoError(t, err)
	_, err = parseAndValidate("./testdata/doesnotexist.yaml")
	require.Error(t, err)
	os.Setenv("RPM_KEY_FILE", "my/rpm/key/file")
	os.Setenv("TEST_RELEASE_ENV_VAR", "1234")
	os.Setenv("TEST_PRERELEASE_ENV_VAR", "beta1")
	config, err := parseAndValidate("./testdata/env-fields.yaml")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("v%s", os.Getenv("GOROOT")), config.Version)
	require.Equal(t, "1234", config.Release)
	require.Equal(t, "beta1", config.Prerelease)
	require.Equal(t, "my/rpm/key/file", config.RPM.Signature.KeyFile)
	require.Equal(t, "hard/coded/file", config.Deb.Signature.KeyFile)
	require.Equal(t, "", config.APK.Signature.KeyFile)
}

func TestParseEnhancedFile(t *testing.T) {
	config, err := parseAndValidate("./testdata/contents.yaml")
	require.NoError(t, err)
	require.Equal(t, "contents foo", config.Name)
	shouldFind := 5
	require.Len(t, config.Contents, shouldFind)
}

func TestParseEnhancedNestedGlobFile(t *testing.T) {
	config, err := parseAndValidate("./testdata/contents_glob.yaml")
	require.NoError(t, err)
	shouldFind := 3
	require.Len(t, config.Contents, shouldFind)
}

func TestParseEnhancedNestedNoGlob(t *testing.T) {
	config, err := parseAndValidate("./testdata/contents_directory.yaml")
	require.NoError(t, err)
	shouldFind := 3
	require.Len(t, config.Contents, shouldFind)
	for _, f := range config.Contents {
		switch f.Source {
		case "testdata/globtest/nested/b.txt":
			require.Equal(t, "/etc/foo/nested/b.txt", f.Destination)
		case "testdata/globtest/multi-nested/subdir/c.txt":
			require.Equal(t, "/etc/foo/multi-nested/subdir/c.txt", f.Destination)
		case "testdata/globtest/a.txt":
			require.Equal(t, "/etc/foo/a.txt", f.Destination)
		default:
			t.Errorf("unknown source %s", f.Source)
		}
	}
}

func TestOptionsFromEnvironment(t *testing.T) {
	const (
		globalPass      = "hunter2"
		debPass         = "password123"
		rpmPass         = "secret"
		apkPass         = "foobar"
		platform        = "linux"
		arch            = "amd64"
		release         = "3"
		version         = "1.0.0"
		vendor          = "GoReleaser"
		packager        = "nope"
		maintainerEmail = "nope@example.com"
	)

	t.Run("platform", func(t *testing.T) {
		t.Setenv("OS", platform)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nplatform: $OS"))
		require.NoError(t, err)
		require.Equal(t, platform, info.Platform)
	})

	t.Run("arch", func(t *testing.T) {
		t.Setenv("ARCH", arch)
		info, err := nfpm.Parse(strings.NewReader("name: foo\narch: $ARCH"))
		require.NoError(t, err)
		require.Equal(t, arch, info.Arch)
	})

	t.Run("version", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("VERSION", version)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nversion: $VERSION"))
		require.NoError(t, err)
		require.Equal(t, version, info.Version)
	})

	t.Run("release", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("RELEASE", release)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nrelease: $RELEASE"))
		require.NoError(t, err)
		require.Equal(t, release, info.Release)
	})

	t.Run("maintainer", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("GIT_COMMITTER_NAME", packager)
		os.Setenv("GIT_COMMITTER_EMAIL", maintainerEmail)
		info, err := nfpm.Parse(strings.NewReader(`
name: foo
maintainer: '"$GIT_COMMITTER_NAME" <$GIT_COMMITTER_EMAIL>'
`))
		require.NoError(t, err)
		addr := mail.Address{
			Name:    packager,
			Address: maintainerEmail,
		}
		require.Equal(t, addr.String(), info.Maintainer)
	})

	t.Run("vendor", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("VENDOR", vendor)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nvendor: $VENDOR"))
		require.NoError(t, err)
		require.Equal(t, vendor, info.Vendor)
	})

	t.Run("global passphrase", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NFPM_PASSPHRASE", globalPass)
		info, err := nfpm.Parse(strings.NewReader("name: foo"))
		require.NoError(t, err)
		require.Equal(t, globalPass, info.Deb.Signature.KeyPassphrase)
		require.Equal(t, globalPass, info.RPM.Signature.KeyPassphrase)
		require.Equal(t, globalPass, info.APK.Signature.KeyPassphrase)
	})

	t.Run("specific passphrases", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NFPM_PASSPHRASE", globalPass)
		os.Setenv("NFPM_DEB_PASSPHRASE", debPass)
		os.Setenv("NFPM_RPM_PASSPHRASE", rpmPass)
		os.Setenv("NFPM_APK_PASSPHRASE", apkPass)
		info, err := nfpm.Parse(strings.NewReader("name: foo"))
		require.NoError(t, err)
		require.Equal(t, debPass, info.Deb.Signature.KeyPassphrase)
		require.Equal(t, rpmPass, info.RPM.Signature.KeyPassphrase)
		require.Equal(t, apkPass, info.APK.Signature.KeyPassphrase)
	})

	t.Run("packager", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("PACKAGER", packager)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nrpm:\n  packager: $PACKAGER"))
		require.NoError(t, err)
		require.Equal(t, packager, info.RPM.Packager)
	})

	t.Run("depends", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("VERSION", version)
		info, err := nfpm.Parse(strings.NewReader(`---
name: foo
overrides:
  deb:
    depends:
    - package (= ${VERSION})
  rpm:
    depends:
    - package = ${VERSION}`))
		require.NoError(t, err)
		require.Len(t, info.Overrides["deb"].Depends, 1)
		require.Equal(t, "package (= 1.0.0)", info.Overrides["deb"].Depends[0])
		require.Len(t, info.Overrides["rpm"].Depends, 1)
		require.Equal(t, "package = 1.0.0", info.Overrides["rpm"].Depends[0])
	})

	t.Run("depends-strips-empty", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("VERSION", version)
		os.Setenv("PKG", "")
		info, err := nfpm.Parse(strings.NewReader(`---
name: foo
overrides:
  deb:
    depends:
    - ${PKG}
    - package (= ${VERSION})
    - ${PKG}
    - ${PKG}
  rpm:
    depends:
    - package = ${VERSION}
    - ${PKG}`))
		require.NoError(t, err)
		require.Len(t, info.Overrides["deb"].Depends, 1)
		require.Equal(t, "package (= 1.0.0)", info.Overrides["deb"].Depends[0])
		require.Len(t, info.Overrides["rpm"].Depends, 1)
		require.Equal(t, "package = 1.0.0", info.Overrides["rpm"].Depends[0])
	})
}

func TestOverrides(t *testing.T) {
	nfpm.RegisterPackager("deb", &fakePackager{})
	nfpm.RegisterPackager("rpm", &fakePackager{})
	nfpm.RegisterPackager("apk", &fakePackager{})

	file := "./testdata/overrides.yaml"
	config, err := nfpm.ParseFile(file)
	require.NoError(t, err)
	require.Equal(t, "foo", config.Name)
	require.Equal(t, "amd64", config.Arch)

	for _, format := range []string{"apk", "deb", "rpm"} {
		format := format
		t.Run(format, func(t *testing.T) {
			pkg, err := config.Get(format)
			require.NoError(t, err)
			require.Equal(t, pkg.Depends, []string{format + "_depend"})
			for _, f := range pkg.Contents {
				switch f.Packager {
				case format:
					require.Contains(t, f.Destination, "/"+format)
				case "":
					require.True(t, f.Destination == "/etc/foo/whatever.conf")
				default:
					t.Fatalf("invalid packager: %s", f.Packager)
				}
			}
			require.Equal(t, "amd64", pkg.Arch)
		})
	}

	t.Run("no_overrides", func(t *testing.T) {
		info, err := config.Get("doesnotexist")
		require.NoError(t, err)
		require.True(t, reflect.DeepEqual(&config.Info, info))
	})
}

type fakePackager struct{}

func (*fakePackager) ConventionalFileName(info *nfpm.Info) string {
	return ""
}

func (*fakePackager) Package(info *nfpm.Info, w io.Writer) error {
	return nil
}
