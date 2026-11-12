package dmg_test

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/dmg"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/macostest"
	osxdmg "go.digitalxero.dev/osx/dmg"
	"go.digitalxero.dev/osx/sign"
	"golang.org/x/text/unicode/norm"
	"howett.net/plist"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	identifier = "com.example.foo"
	executable = "/Contents/MacOS/foo"
	script     = "#!/bin/sh\necho foo\n"
)

// exampleInfo is the smallest disk image: an application bundle with one
// executable.
func exampleInfo(t *testing.T) *nfpm.Info {
	t.Helper()
	return nfpm.WithDefaults(&nfpm.Info{
		Name:    "foo",
		Arch:    "arm64",
		Version: "1.2.3",
		MTime:   macostest.MTime,
		Overridables: nfpm.Overridables{
			Contents: files.Contents{
				{
					Source:      macostest.WriteFile(t, "foo", script),
					Destination: executable,
					FileInfo:    &files.ContentFileInfo{Mode: 0o755},
				},
			},
			App: nfpm.App{Identifier: identifier},
		},
	})
}

func build(t *testing.T, info *nfpm.Info) []byte {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, dmg.Default.Package(info, &out))
	return out.Bytes()
}

// open builds the image and reads it back, checking its checksums the way
// "hdiutil verify" does.
func open(t *testing.T, info *nfpm.Info) osxdmg.ImageReader {
	t.Helper()
	raw := build(t, info)
	image, err := osxdmg.Open(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	require.NoError(t, image.Verify())
	return image
}

func packageError(t *testing.T, info *nfpm.Info) error {
	t.Helper()
	var out bytes.Buffer
	err := dmg.Default.Package(info, &out)
	require.Error(t, err)
	return err
}

func TestRegistered(t *testing.T) {
	packager, err := nfpm.Get("dmg")
	require.NoError(t, err)
	require.Same(t, dmg.Default, packager)
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".dmg", dmg.Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo(t)
	require.Equal(t, "foo_1.2.3_arm64.dmg", dmg.Default.ConventionalFileName(info))

	info.Prerelease = "rc1"
	info.App.Arch = "universal"
	require.Equal(t, "foo_1.2.3-rc1_universal.dmg", dmg.Default.ConventionalFileName(info))
}

// The image holds the application bundle and a link to the Applications
// folder to drag it to, and nothing else.
func TestImage(t *testing.T) {
	info := exampleInfo(t)
	info.App.Name = "Foo App"
	info.App.MinOSVersion = "13.0"
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      macostest.WriteFile(t, "data", "data\n"),
			Destination: "/Contents/Resources/data",
			FileInfo:    &files.ContentFileInfo{Mode: 0o640},
		},
		&files.Content{
			Source:      "../MacOS/foo",
			Destination: "/Contents/Resources/launcher",
			Type:        files.TypeSymlink,
		},
	)
	image := open(t, info)
	volume := image.FS()

	assert.Equal(t, "Foo App", image.VolumeName())
	assert.True(t, macostest.MTime.Equal(image.Created()), image.Created())
	assert.False(t, image.IsSigned())

	top, err := fs.ReadDir(volume, ".")
	require.NoError(t, err)
	names := make([]string, 0, len(top))
	for _, entry := range top {
		names = append(names, entry.Name())
	}
	assert.ElementsMatch(t, []string{"Applications", "Foo App.app"}, names)
	assert.Equal(t, "/Applications", macostest.ReadLink(t, volume, "Applications"))

	main := macostest.Lstat(t, volume, "Foo App.app/Contents/MacOS/foo")
	assert.Equal(t, fs.FileMode(0o755), main.Mode(), "the executable bit must survive the image")
	assert.True(t, macostest.MTime.Equal(main.ModTime()), main.ModTime())
	assert.Equal(t, script, string(macostest.ReadFile(t, volume, "Foo App.app/Contents/MacOS/foo")))
	assert.Equal(t, "data\n", string(macostest.ReadFile(t, volume, "Foo App.app/Contents/Resources/data")))
	assert.Equal(t, fs.FileMode(0o640), macostest.Lstat(t, volume, "Foo App.app/Contents/Resources/data").Mode())
	assert.Equal(t, "../MacOS/foo", macostest.ReadLink(t, volume, "Foo App.app/Contents/Resources/launcher"))
	assert.Equal(t, "APPL????", string(macostest.ReadFile(t, volume, "Foo App.app/Contents/PkgInfo")))

	var values map[string]any
	_, err = plist.Unmarshal(macostest.ReadFile(t, volume, "Foo App.app/Contents/Info.plist"), &values)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"CFBundleExecutable":            "foo",
		"CFBundleIdentifier":            identifier,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleName":                  "Foo App",
		"CFBundlePackageType":           "APPL",
		"CFBundleShortVersionString":    "1.2.3",
		"CFBundleVersion":               "1.2.3",
		"LSMinimumSystemVersion":        "13.0",
	}, values)
}

// Contents meant for the zipped bundle only stay out of the image, and the
// other way around: dmg is a packager of its own.
func TestContentsArePerPackager(t *testing.T) {
	info := exampleInfo(t)
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      macostest.WriteFile(t, "dmg", "dmg\n"),
			Destination: "/Contents/Resources/only-dmg",
			Packager:    "dmg",
		},
		&files.Content{
			Source:      macostest.WriteFile(t, "app", "app\n"),
			Destination: "/Contents/Resources/only-app",
			Packager:    "app",
		},
	)
	volume := open(t, info).FS()
	assert.Equal(t, "dmg\n", string(macostest.ReadFile(t, volume, "foo.app/Contents/Resources/only-dmg")))
	_, err := fs.Stat(volume, "foo.app/Contents/Resources/only-app")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// The bundle rules are the app packager's; a few of them are checked here to
// prove the image is built through them.
func TestBundleRulesApply(t *testing.T) {
	t.Run("destination outside the bundle", func(t *testing.T) {
		info := exampleInfo(t)
		info.Contents[0].Destination = "/usr/local/bin/foo"
		require.ErrorContains(t, packageError(t, info), "start with /Contents/")
	})
	t.Run("missing identifier", func(t *testing.T) {
		info := exampleInfo(t)
		info.App.Identifier = ""
		require.ErrorContains(t, packageError(t, info), "package app.identifier must be provided")
	})
	t.Run("unsupported architecture", func(t *testing.T) {
		info := exampleInfo(t)
		info.Arch = "386"
		require.ErrorContains(t, packageError(t, info), `package arch "386" is not supported by macOS`)
	})
	t.Run("wrong architecture", func(t *testing.T) {
		info := exampleInfo(t)
		info.Contents[0].Source = macostest.Program(t, "amd64")
		require.ErrorContains(t, packageError(t, info), "is not built for arm64")
	})
	t.Run("warnings name the packager", func(t *testing.T) {
		logs := macostest.CaptureLogs(t)
		info := exampleInfo(t)
		info.Depends = []string{"bash"}
		build(t, info)
		require.Contains(t, logs.String(), "warning: dmg: macOS has no package relationships, depends is ignored")
	})
}

func TestNonASCIIName(t *testing.T) {
	info := exampleInfo(t)
	info.App.Name = "Café 世界"
	image := open(t, info)
	// HFS+ stores names decomposed, which is how macOS itself writes them.
	assert.Equal(t, norm.NFD.String("Café 世界"), image.VolumeName())
	body := macostest.ReadFile(t, image.FS(), norm.NFD.String("Café 世界.app/Contents/MacOS/foo"))
	require.Equal(t, script, string(body))
}

func TestReproducible(t *testing.T) {
	info := exampleInfo(t)
	first := build(t, info)
	require.Equal(t, first, build(t, info), "the same input must give the same bytes")

	require.NoError(t, os.WriteFile(info.Contents[0].Source, []byte("#!/bin/sh\necho changed\n"), 0o644))
	require.NotEqual(t, first, build(t, info), "changed content must change the image")
}

func TestPackageDoesNotChangeTheInfo(t *testing.T) {
	info := exampleInfo(t)
	info.Arch = "amd64"
	contents := info.Contents
	build(t, info)
	require.Equal(t, "amd64", info.Arch)
	require.Equal(t, contents, info.Contents)
}

func signedInfo(t *testing.T, p12 string) *nfpm.Info {
	t.Helper()
	info := exampleInfo(t)
	info.Contents[0].Source = macostest.Program(t, "arm64")
	info.App.Signature = nfpm.AppSignature{
		P12File:       p12,
		TimestampURL:  "none",
		KeyPassphrase: macostest.Passphrase,
	}
	return info
}

// One identity signs both the bundle and the image that carries it.
func TestSigning(t *testing.T) {
	p12, certificate := macostest.Identity(t)
	image := open(t, signedInfo(t, p12))

	require.True(t, image.IsSigned())
	require.NotEmpty(t, image.Certificates())
	require.Equal(t, certificate.Raw, image.Certificates()[0])
	require.True(t, bytes.Contains(image.CodeSignature(), certificate.Raw))

	volume := image.FS()
	program := macostest.CodeSignature(t, macostest.ReadFile(t, volume, "foo.app/Contents/MacOS/foo"))
	require.True(t, bytes.Contains(program, certificate.Raw), "the bundle inside the image is not signed")
	_, err := fs.Stat(volume, "foo.app/Contents/_CodeSignature/CodeResources")
	require.NoError(t, err)
}

func TestSigningWrongPassphrase(t *testing.T) {
	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.App.Signature.KeyPassphrase = "wrong"
	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, packageError(t, info), &signing)
}

// A timestamp service that fails is a signing failure, and callers can tell.
func TestSigningTimestampFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.App.Signature.TimestampURL = server.URL
	err := packageError(t, info)

	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &signing)
	require.ErrorIs(t, err, sign.ErrTimestamp)
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// The default is Apple's timestamp service, as codesign does, and nothing is
// written when signing fails. The request is intercepted, so the test proves
// where it would go without going there.
func TestSigningUsesApplesTimestampServiceByDefault(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	var requested string
	http.DefaultTransport = roundTripper(func(request *http.Request) (*http.Response, error) {
		requested = request.URL.String()
		return nil, errors.New("intercepted")
	})

	p12, _ := macostest.Identity(t)
	info := signedInfo(t, p12)
	info.App.Signature.TimestampURL = ""
	var out bytes.Buffer
	err := dmg.Default.Package(info, &out)

	var signing *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &signing)
	require.ErrorIs(t, err, sign.ErrTimestamp)
	require.Contains(t, requested, "timestamp.apple.com")
	require.Zero(t, out.Len(), "nothing is written when signing fails")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// An image that cannot be written is not a signing problem.
func TestWriteFailureIsNotASigningFailure(t *testing.T) {
	p12, _ := macostest.Identity(t)
	err := dmg.Default.Package(signedInfo(t, p12), failingWriter{})
	require.ErrorContains(t, err, "disk full")
	var signing *nfpm.ErrSigningFailure
	require.NotErrorAs(t, err, &signing)

	err = dmg.Default.Package(exampleInfo(t), failingWriter{})
	require.ErrorContains(t, err, "disk full")
	require.NotErrorAs(t, err, &signing)
}

func TestCertificateKindWarning(t *testing.T) {
	logs := macostest.CaptureLogs(t)
	p12, _ := macostest.Identity(t)
	build(t, signedInfo(t, p12))
	require.Contains(t, logs.String(), "warning: dmg: the certificate")
	require.Contains(t, logs.String(), "is not a Developer ID Application certificate")

	logs.Reset()
	p12, _ = macostest.Identity(t, macostest.DeveloperIDApplication)
	build(t, signedInfo(t, p12))
	require.Empty(t, logs.String())
}
