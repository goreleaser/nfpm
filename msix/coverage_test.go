package msix

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureValidArchUnknown(t *testing.T) {
	info := &nfpm.Info{Arch: "riscv64"}
	require.Equal(t, "riscv64", ensureValidArch(info).Arch,
		"an arch absent from archToMSIX must pass through untouched")
}

func TestEnsureDN(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   string
		want string
	}{
		{"plain name", "TestCo", "CN=TestCo"},
		{"already a DN", "CN=TestCo, O=TestCo", "CN=TestCo, O=TestCo"},
		{"already a DN with a dotted attribute", "OID.2.5.4.3=TestCo", "OID.2.5.4.3=TestCo"},
		{"a leading digit is not an attribute", "1.2.3=x", `CN="1.2.3=x"`},
		{"comma is quoted", "Test, Inc", `CN="Test, Inc"`},
		{"plus is quoted", "Test+Co", `CN="Test+Co"`},
		{"quote is doubled", `Test"Co`, `CN="Test""Co"`},
		{"backslash is literal", `Test\Co`, `CN="Test\Co"`},
		{"angle brackets are quoted", "Test<Co>", `CN="Test<Co>"`},
		{"semicolon is quoted", "Test;Co", `CN="Test;Co"`},
		{"leading space is quoted", " TestCo", `CN=" TestCo"`},
		{"trailing space is quoted", "TestCo ", `CN="TestCo "`},
		{"interior space is not escaped", "Test Co", "CN=Test Co"},
		{"leading hash is quoted", "#TestCo", `CN="#TestCo"`},
		{"interior hash is quoted", "Test#Co", `CN="Test#Co"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ensureDN(tt.in))
		})
	}
}

func TestConvertToMSIXVersionEdgeCases(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"1.2.3.4", "1.2.3.4"},
		{"1.2.3", "1.2.3.0"},
		{"1", "1.0.0.0"},
		{"", "0.0.0.0"},
		{"v1.2.3.4", "1.2.3.4"},
		{"1.x.3.4", "1.0.3.4"},
		{"notanumber", "0.0.0.0"},
		{"1.2.3-rc1", "1.2.0.0"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, convertToMSIXVersion(tt.in))
		})
	}
}

func TestNormalizePathForMSIX(t *testing.T) {
	require.Equal(t, "app/fake.exe", normalizePathForMSIX("/app/fake.exe"))
	require.Equal(t, "app/fake.exe", normalizePathForMSIX("app/fake.exe"))
}

// TestSetPackagerDefaultsIdempotent proves an explicit runFullTrust capability
// is not duplicated when the defaults are applied.
func TestSetPackagerDefaultsIdempotent(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Capabilities.Restricted = []string{"runFullTrust"}

	Default.SetPackagerDefaults(info)
	require.Equal(t, []string{"runFullTrust"}, info.MSIX.Capabilities.Restricted)

	// Applying the defaults twice must not append a second entry either.
	Default.SetPackagerDefaults(info)
	require.Equal(t, []string{"runFullTrust"}, info.MSIX.Capabilities.Restricted)
}

// TestSetPackagerDefaultsNoLogo covers the guards that leave the per-app logos
// empty when the package has no logo. Package cannot reach this state because
// validate rejects an empty logo, so the defaults are applied directly.
func TestSetPackagerDefaultsNoLogo(t *testing.T) {
	info := &nfpm.Info{
		Name: "MyCompany.TestApp",
		Overridables: nfpm.Overridables{
			MSIX: nfpm.MSIX{
				Applications: []nfpm.MSIXApplication{{ID: "App", Executable: "app/fake.exe"}},
			},
		},
	}
	Default.SetPackagerDefaults(info)

	require.Empty(t, info.MSIX.Applications[0].VisualElements.Square150x150Logo)
	require.Empty(t, info.MSIX.Applications[0].VisualElements.Square44x44Logo)
	require.Equal(t, "Windows.FullTrustApplication", info.MSIX.Applications[0].EntryPoint)
	require.Equal(t, "transparent", info.MSIX.Applications[0].VisualElements.BackgroundColor)
	require.Equal(t, []string{"runFullTrust"}, info.MSIX.Capabilities.Restricted)
}

// TestSetPackagerDefaultsNoFullTrust covers the arm where no application needs
// full trust, so no restricted capability is added.
func TestSetPackagerDefaultsNoFullTrust(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Applications[0].EntryPoint = "MyApp.Main"

	Default.SetPackagerDefaults(info)
	require.Empty(t, info.MSIX.Capabilities.Restricted)
}

// TestPublisherDisplayNameFallbacks covers the display-name fallback chain.
// go-msix exposes Properties as an opaque interface, so the built manifest is
// the observable surface.
func TestPublisherDisplayNameFallbacks(t *testing.T) {
	t.Run("falls back to vendor", func(t *testing.T) {
		info := exampleInfo()
		info.MSIX.Properties.PublisherDisplayName = ""

		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		require.Contains(t, readManifest(t, buf.Bytes()),
			"<PublisherDisplayName>TestCo</PublisherDisplayName>")
	})

	t.Run("falls back to package name", func(t *testing.T) {
		// An explicit publisher is required here: validate rejects an empty one,
		// which is what an empty vendor and maintainer would otherwise produce.
		info := exampleInfo()
		info.MSIX.Properties.PublisherDisplayName = ""
		info.Vendor = ""
		info.Maintainer = ""

		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		require.Contains(t, readManifest(t, buf.Bytes()),
			"<PublisherDisplayName>MyCompany.TestApp</PublisherDisplayName>")
	})

	t.Run("display name falls back to package name", func(t *testing.T) {
		info := exampleInfo()
		info.MSIX.Properties.DisplayName = ""

		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		require.Contains(t, readManifest(t, buf.Bytes()),
			"<DisplayName>MyCompany.TestApp</DisplayName>")
	})
}

// TestApplicationVisualElementsFallbacks proves an application with no visual
// metadata inherits the package display name and description.
func TestApplicationVisualElementsFallbacks(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Applications[0].VisualElements.DisplayName = ""
	info.MSIX.Applications[0].VisualElements.Description = ""

	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	manifest := readManifest(t, buf.Bytes())
	require.Contains(t, manifest, `DisplayName="MyCompany.TestApp"`)
	require.Contains(t, manifest, `Description="Test application"`)
}

func TestAddContentsSkips(t *testing.T) {
	t.Run("directories are skipped", func(t *testing.T) {
		info := exampleInfo()
		info.Contents = append(info.Contents, &files.Content{
			Destination: "/app/data",
			Type:        files.TypeDir,
		})

		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
	})

	t.Run("symlinks are skipped with a warning", func(t *testing.T) {
		var logs bytes.Buffer
		log.SetOutput(&logs)
		t.Cleanup(func() { log.SetOutput(os.Stderr) })

		info := exampleInfo()
		info.Contents = append(info.Contents, &files.Content{
			Source:      "/app/fake.exe",
			Destination: "/app/link.exe",
			Type:        files.TypeSymlink,
		})

		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		require.Contains(t, logs.String(), "msix does not support symlinks")
		require.NotContains(t, readManifest(t, buf.Bytes()), "link.exe")
	})

	// Package cannot reach the empty-source guard: nfpm's globbing rejects a
	// content with no source first. Passing a nil builder proves AddFile is
	// never called for such an entry — it would panic if it were.
	t.Run("contents without a source are dropped", func(t *testing.T) {
		info := &nfpm.Info{
			Overridables: nfpm.Overridables{
				Contents: []*files.Content{{Destination: "/app/ghost.exe"}},
			},
		}
		require.NotPanics(t, func() { addContents(nil, info) })
	})
}

// TestBuildError proves errors deferred by AddFile surface from Build. The
// builder does not open sources until it writes the package.
func TestBuildError(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "/does/not/exist/app.exe",
		Destination: "/app/missing.exe",
	})

	err := Default.Package(info, io.Discard)
	require.Error(t, err)
}

func TestConfigureSigningMissingPFX(t *testing.T) {
	info := exampleInfo()
	info.MSIX.Signature.PFXFile = "/does/not/exist.pfx"

	err := Default.Package(info, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PFX file not found")
}

// TestConfigureSigningPFXNotAccessible covers the stat failure that is not
// ErrNotExist: a path whose parent is a regular file yields ENOTDIR on unix.
// Windows reports that as ERROR_PATH_NOT_FOUND, which maps to fs.ErrNotExist
// and takes the not-found branch instead.
func TestConfigureSigningPFXNotAccessible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows maps a non-directory path component to ErrNotExist")
	}

	info := exampleInfo()
	info.MSIX.Signature.PFXFile = filepath.Join("../testdata/whatever.conf", "nope.pfx")

	err := Default.Package(info, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to access PFX file")
}

func TestConfigureSigningWrongPassphrase(t *testing.T) {
	pfxPath, _ := makeTestPFX(t)

	info := exampleInfo()
	info.MSIX.Signature = nfpm.MSIXSignature{
		PFXFile:       pfxPath,
		KeyPassphrase: "wrong-passphrase",
	}

	err := Default.Package(info, io.Discard)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &expectedError)
}

// TestPublisherFromBadPFX ensures signing failures are reported even when
// no publisher fallback is available.
func TestPublisherFromBadPFX(t *testing.T) {
	pfxPath := filepath.Join(t.TempDir(), "corrupt.pfx")
	require.NoError(t, os.WriteFile(pfxPath, []byte("not a pfx"), 0o600))
	validPFX, _ := makeTestPFX(t)

	for _, tt := range []struct {
		name       string
		path       string
		passphrase string
	}{
		{name: "corrupt", path: pfxPath},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing.pfx")},
		{name: "wrong passphrase", path: validPFX, passphrase: "wrong-passphrase"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, fallback := range []struct {
				name      string
				vendor    string
				publisher string
			}{
				{name: "vendor fallback", vendor: "FallbackCo", publisher: "CN=FallbackCo"},
				{name: "no fallback"},
			} {
				t.Run(fallback.name, func(t *testing.T) {
					info := exampleInfo()
					info.MSIX.Publisher = ""
					info.Vendor = fallback.vendor
					info.Maintainer = ""
					info.MSIX.Signature = nfpm.MSIXSignature{
						PFXFile:       tt.path,
						KeyPassphrase: tt.passphrase,
					}

					err := Default.Package(info, io.Discard)
					require.Error(t, err)
					if tt.name == "missing" && fallback.vendor != "" {
						require.ErrorIs(t, err, os.ErrNotExist)
						assert.Contains(t, err.Error(), "PFX file not found")
					} else {
						var expectedError *nfpm.ErrSigningFailure
						require.ErrorAs(t, err, &expectedError)
						assert.Contains(t, err.Error(), "loading PFX")
					}
					assert.NotContains(t, err.Error(), "msix.publisher, vendor, or maintainer")
					assert.Equal(t, fallback.publisher, info.MSIX.Publisher)
				})
			}
		})
	}
}
