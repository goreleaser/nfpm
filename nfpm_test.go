package nfpm_test

import (
	"fmt"
	"io"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
)

var mtime = time.Date(2023, 11, 5, 23, 15, 17, 0, time.UTC)

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
	info := nfpm.WithDefaults(&nfpm.Info{
		Version:       "v1.0.0",
		VersionSchema: "semver",
	})
	require.NotEmpty(t, info.Platform)
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "", info.Release)
	require.Equal(t, "", info.Prerelease)

	info = nfpm.WithDefaults(&nfpm.Info{
		Version: "v1.0.0-rc1",
	})
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "", info.Release)
	require.Equal(t, "rc1", info.Prerelease)

	info = nfpm.WithDefaults(&nfpm.Info{
		Version: "v1.0.0-beta1",
	})
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "", info.Release)
	require.Equal(t, "beta1", info.Prerelease)

	info = nfpm.WithDefaults(&nfpm.Info{
		Version:    "v1.0.0-1",
		Release:    "2",
		Prerelease: "beta1",
	})
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "2", info.Release)
	require.Equal(t, "beta1", info.Prerelease)

	info = nfpm.WithDefaults(&nfpm.Info{
		Version:    "v1.0.0-1+xdg2",
		Release:    "2",
		Prerelease: "beta1",
	})
	require.Equal(t, "1.0.0", info.Version)
	require.Equal(t, "2", info.Release)
	require.Equal(t, "beta1", info.Prerelease)

	info = nfpm.WithDefaults(&nfpm.Info{
		Version:       "this.is.my.version",
		VersionSchema: "none",
		Release:       "2",
		Prerelease:    "beta1",
	})
	require.Equal(t, "this.is.my.version", info.Version)
	require.Equal(t, "2", info.Release)
	require.Equal(t, "beta1", info.Prerelease)
}

func TestDefaults(t *testing.T) {
	t.Run("all given", func(t *testing.T) {
		makeinfo := func() nfpm.Info {
			return nfpm.Info{
				Platform:    "darwin",
				Version:     "2.4.1",
				Description: "no description given",
				Arch:        "arm64",
				MTime:       mtime,
				Overridables: nfpm.Overridables{
					Umask: 0o112,
				},
			}
		}
		info := makeinfo()
		nfpm.WithDefaults(&info)
		require.Equal(t, makeinfo(), info)
	})
	t.Run("none given", func(t *testing.T) {
		t.Setenv("SOURCE_DATE_EPOCH", strconv.FormatInt(mtime.Unix(), 10))
		got := nfpm.WithDefaults(&nfpm.Info{})
		require.Equal(t, nfpm.Info{
			Platform:    "linux",
			Arch:        "amd64",
			Version:     "0.0.0",
			Prerelease:  "rc0",
			Description: "no description given",
			MTime:       mtime,
			Overridables: nfpm.Overridables{
				Umask: 0o002,
			},
		}, *got)
	})
	t.Run("mips softfloat", func(t *testing.T) {
		makeinfo := func() nfpm.Info {
			return nfpm.Info{
				Platform: "linux",
				Arch:     "mips64softfloat",
			}
		}
		info := makeinfo()
		nfpm.WithDefaults(&info)
		require.Equal(t, "mips64", info.Arch)
	})
	t.Run("mips softfloat", func(t *testing.T) {
		makeinfo := func() nfpm.Info {
			return nfpm.Info{
				Platform: "linux",
				Arch:     "mips64hardfloat",
			}
		}
		info := makeinfo()
		nfpm.WithDefaults(&info)
		require.Equal(t, "mips64", info.Arch)
	})
}

func TestPrepareForPackager(t *testing.T) {
	t.Run("dirs", func(t *testing.T) {
		info := nfpm.WithDefaults(&nfpm.Info{
			Name:    "as",
			Arch:    "asd",
			Version: "1.2.3",
			Overridables: nfpm.Overridables{
				Umask: 0o032,
				Contents: []*files.Content{
					{
						Destination: "/usr/share/test",
						Type:        files.TypeDir,
					},
					{
						Source:      "./testdata/contents.yaml",
						Destination: "asd",
					},
					{
						Destination: "/usr/a",
						Type:        files.TypeDir,
					},
				},
			},
		})
		require.NoError(t, nfpm.PrepareForPackager(info, ""))
		require.Len(t, info.Overridables.Contents, 5)
		asdFile := info.Overridables.Contents[0]
		require.Equal(t, "/asd", asdFile.Destination)
		require.Equal(t, files.TypeFile, asdFile.Type)
		require.Equal(t, "-rw-r--r--", asdFile.FileInfo.Mode.String())
		require.Equal(t, "root", asdFile.FileInfo.Owner)
		require.Equal(t, "root", asdFile.FileInfo.Group)
		usrDir := info.Overridables.Contents[1]
		require.Equal(t, "/usr/", usrDir.Destination)
		require.Equal(t, files.TypeImplicitDir, usrDir.Type)
		require.Equal(t, "-rwxr-xr-x", usrDir.FileInfo.Mode.String())
		require.Equal(t, "root", usrDir.FileInfo.Owner)
		require.Equal(t, "root", usrDir.FileInfo.Group)
		aDir := info.Overridables.Contents[2]
		require.Equal(t, "/usr/a/", aDir.Destination)
		require.Equal(t, files.TypeDir, aDir.Type)
		require.Equal(t, "-rwxr-xr-x", aDir.FileInfo.Mode.String())
		require.Equal(t, "root", aDir.FileInfo.Owner)
		require.Equal(t, "root", aDir.FileInfo.Group)
	})

	t.Run("config", func(t *testing.T) {
		require.NoError(t, nfpm.PrepareForPackager(&nfpm.Info{
			Name:    "as",
			Arch:    "asd",
			Version: "1.2.3",
			Overridables: nfpm.Overridables{
				Contents: []*files.Content{
					{
						Source:      "./testdata/contents.yaml",
						Destination: "asd",
						Type:        files.TypeConfig,
					},
				},
			},
		}, ""))
	})
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
						Type:        files.TypeDir,
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
						Type:        files.TypeConfig,
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
	if err != nil {
		return config, fmt.Errorf("parse file: %w", err)
	}

	err = config.Validate()
	if err != nil {
		return config, fmt.Errorf("validate: %w", err)
	}

	err = nfpm.PrepareForPackager(&config.Info, "")
	if err != nil {
		return config, fmt.Errorf("prepare for packager: %w", err)
	}

	return config, nil
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
	t.Setenv("RPM_KEY_FILE", "my/rpm/key/file")
	t.Setenv("TEST_RELEASE_ENV_VAR", "1234")
	t.Setenv("TEST_PRERELEASE_ENV_VAR", "beta1")
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
	shouldFind := 10
	require.Len(t, config.Contents, shouldFind)
}

func TestParseEnhancedNestedGlobFile(t *testing.T) {
	config, err := parseAndValidate("./testdata/contents_glob.yaml")
	require.NoError(t, err)
	shouldFind := 5
	require.Len(t, config.Contents, shouldFind)
}

func TestParseEnhancedNestedNoGlob(t *testing.T) {
	config, err := parseAndValidate("./testdata/contents_directory.yaml")
	require.NoError(t, err)
	shouldFind := 8
	require.Len(t, config.Contents, shouldFind)
	tested := 0
	for _, f := range config.Contents {
		if f.Type == files.TypeImplicitDir {
			continue
		}

		switch f.Source {
		case "testdata/globtest/nested/b.txt":
			tested++
			require.Equal(t, "/etc/foo/nested/b.txt", f.Destination)
		case "testdata/globtest/multi-nested/subdir/c.txt":
			tested++
			require.Equal(t, "/etc/foo/multi-nested/subdir/c.txt", f.Destination)
		case "testdata/globtest/a.txt":
			tested++
			require.Equal(t, "/etc/foo/a.txt", f.Destination)
		default:
			t.Errorf("unknown source %q", f.Source)
		}
	}
	require.Equal(t, 3, tested)
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
		homepage        = "https://nfpm.goreleaser.com"
		vcsBrowser      = "https://github.com/goreleaser/nfpm"
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
		t.Setenv("VERSION", version)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nversion: $VERSION"))
		require.NoError(t, err)
		require.Equal(t, version, info.Version)
	})

	t.Run("release", func(t *testing.T) {
		t.Setenv("RELEASE", release)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nrelease: $RELEASE"))
		require.NoError(t, err)
		require.Equal(t, release, info.Release)
	})

	t.Run("maintainer", func(t *testing.T) {
		t.Setenv("GIT_COMMITTER_NAME", packager)
		t.Setenv("GIT_COMMITTER_EMAIL", maintainerEmail)
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
		t.Setenv("VENDOR", vendor)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nvendor: $VENDOR"))
		require.NoError(t, err)
		require.Equal(t, vendor, info.Vendor)
	})

	t.Run("homepage", func(t *testing.T) {
		t.Setenv("CI_PROJECT_URL", homepage)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nhomepage: $CI_PROJECT_URL"))
		require.NoError(t, err)
		require.Equal(t, homepage, info.Homepage)
	})

	t.Run("global passphrase", func(t *testing.T) {
		t.Setenv("NFPM_PASSPHRASE", globalPass)
		info, err := nfpm.Parse(strings.NewReader("name: foo"))
		require.NoError(t, err)
		require.Equal(t, globalPass, info.Deb.Signature.KeyPassphrase)
		require.Equal(t, globalPass, info.RPM.Signature.KeyPassphrase)
		require.Equal(t, globalPass, info.APK.Signature.KeyPassphrase)
	})

	t.Run("specific passphrases", func(t *testing.T) {
		t.Setenv("NFPM_PASSPHRASE", globalPass)
		t.Setenv("NFPM_DEB_PASSPHRASE", debPass)
		t.Setenv("NFPM_RPM_PASSPHRASE", rpmPass)
		t.Setenv("NFPM_APK_PASSPHRASE", apkPass)
		info, err := nfpm.Parse(strings.NewReader("name: foo"))
		require.NoError(t, err)
		require.Equal(t, debPass, info.Deb.Signature.KeyPassphrase)
		require.Equal(t, rpmPass, info.RPM.Signature.KeyPassphrase)
		require.Equal(t, apkPass, info.APK.Signature.KeyPassphrase)
	})

	t.Run("packager", func(t *testing.T) {
		t.Setenv("PACKAGER", packager)
		info, err := nfpm.Parse(strings.NewReader("name: foo\nrpm:\n  packager: $PACKAGER"))
		require.NoError(t, err)
		require.Equal(t, packager, info.RPM.Packager)
	})

	t.Run("depends", func(t *testing.T) {
		t.Setenv("VERSION", version)
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
		t.Setenv("VERSION", version)
		t.Setenv("PKG", "")
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

	t.Run("deb fields", func(t *testing.T) {
		t.Setenv("CI_PROJECT_URL", vcsBrowser)
		info, err := nfpm.Parse(strings.NewReader(`
name: foo
deb:
  fields:
    Vcs-Browser: ${CI_PROJECT_URL}
`))
		require.NoError(t, err)
		require.Equal(t, vcsBrowser, info.Deb.Fields["Vcs-Browser"])
	})

	t.Run("contents", func(t *testing.T) {
		t.Setenv("ARCH", "amd64")
		t.Setenv("NAME", "foo")
		info, err := nfpm.Parse(strings.NewReader(`
name: foo
contents:
- src: '${NAME}_${ARCH}'
  dst: /usr/bin/${NAME}
  expand: true
- src: '${NAME}'
  dst: /usr/bin/bar

overrides:
  deb:
    contents:
    - src: '${NAME}_${ARCH}'
      dst: /debian/usr/bin/${NAME}
      expand: true
`))
		require.NoError(t, err)
		require.Equal(t, 2, info.Contents.Len())
		content1 := info.Contents[0]
		require.Equal(t, "/usr/bin/foo", content1.Destination)
		require.Equal(t, "foo_amd64", content1.Source)
		content2 := info.Contents[1]
		require.Equal(t, "/usr/bin/bar", content2.Destination)
		require.Equal(t, "${NAME}", content2.Source)
		content3 := info.Overrides["deb"].Contents[0]
		require.Equal(t, "/debian/usr/bin/foo", content3.Destination)
		require.Equal(t, "foo_amd64", content3.Source)
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
			require.Equal(t, time.Date(2023, 0o1, 0o2, 0, 0, 0, 0, time.UTC), pkg.MTime)
		})
	}

	t.Run("no_overrides", func(t *testing.T) {
		pkg, err := config.Get("doesnotexist")
		require.NoError(t, err)
		require.Empty(t, pkg.Depends)
	})
}

type fakePackager struct{}

func (*fakePackager) ConventionalFileName(_ *nfpm.Info) string {
	return ""
}

func (*fakePackager) Package(_ *nfpm.Info, _ io.Writer) error {
	return nil
}
