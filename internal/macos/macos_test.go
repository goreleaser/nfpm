package macos_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/goreleaser/nfpm/v2/internal/macos"
	"github.com/goreleaser/nfpm/v2/internal/macostest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveArch(t *testing.T) {
	for shared, want := range map[string]string{
		"amd64": "x86_64", "x86_64": "x86_64", "arm64": "arm64", "aarch64": "arm64",
		"AMD64": "x86_64", "all": "universal", "universal": "universal",
	} {
		got, err := macos.ResolveArch("pkg.arch", "", shared)
		require.NoError(t, err, shared)
		assert.Equal(t, want, got, shared)
	}

	// An explicit value wins and is taken as it is.
	got, err := macos.ResolveArch("pkg.arch", "universal", "arm64")
	require.NoError(t, err)
	assert.Equal(t, "universal", got)

	_, err = macos.ResolveArch("pkg.arch", "", "386")
	require.ErrorContains(t, err, `package arch "386" is not supported by macOS`)
	require.ErrorContains(t, err, "set pkg.arch")
	_, err = macos.ResolveArch("app.arch", "amd64", "arm64")
	require.ErrorContains(t, err, `package app.arch "amd64" is not supported`)
}

func TestHostArchitectures(t *testing.T) {
	assert.Equal(t, []string{"arm64"}, macos.HostArchitectures("arm64"))
	assert.Equal(t, []string{"x86_64"}, macos.HostArchitectures("x86_64"))
	assert.Equal(t, []string{"arm64", "x86_64"}, macos.HostArchitectures("universal"))
}

func TestPathIndex(t *testing.T) {
	index := macos.NewPathIndex()
	require.NoError(t, index.Add("/opt/Foo/bin/tool"))
	require.NoError(t, index.Add("/opt/Foo/bin/tool"), "the same spelling is the same path")
	require.NoError(t, index.Add("/opt/Foo/lib/other"))

	require.ErrorContains(t, index.Add("/opt/Foo/bin/TOOL"), "macOS stores them as the same path")
	// A parent directory in another spelling collides just the same.
	require.ErrorContains(t, index.Add("/opt/foo/share/doc"), "macOS stores them as the same path")
	require.NoError(t, index.Add("/opt/Foo/café"))
	require.ErrorContains(t, index.Add("/opt/Foo/café"), "macOS stores them as the same path")
}

func TestPathKeyIgnoresCaseAndNormalization(t *testing.T) {
	assert.Equal(t, macos.PathKey("/opt/CAFÉ"), macos.PathKey("/opt/café"))
	assert.NotEqual(t, macos.PathKey("/opt/cafe"), macos.PathKey("/opt/café"))
}

func TestTimestampURL(t *testing.T) {
	for _, tc := range []struct {
		setting, url string
		override     bool
	}{
		{"", "", false},
		{"none", "", true},
		{"http://timestamp.example.com", "http://timestamp.example.com", true},
	} {
		url, override := macos.TimestampURL(tc.setting)
		assert.Equal(t, tc.url, url, tc.setting)
		assert.Equal(t, tc.override, override, tc.setting)
	}
}

func TestReadP12(t *testing.T) {
	path := macostest.WriteFile(t, "identity.p12", "identity")
	data, err := macos.ReadP12(path)
	require.NoError(t, err)
	assert.Equal(t, "identity", string(data))

	_, err = macos.ReadP12(filepath.Join(t.TempDir(), "missing.p12"))
	require.ErrorContains(t, err, "p12 file not found")
	require.ErrorIs(t, err, fs.ErrNotExist)

	// A path that exists but cannot be read as a file.
	_, err = macos.ReadP12(t.TempDir())
	require.ErrorContains(t, err, "unable to access p12 file")
}

func TestWarnCertificateKind(t *testing.T) {
	_, plain := macostest.Identity(t)
	_, application := macostest.Identity(t, macostest.DeveloperIDApplication)
	_, installer := macostest.Identity(t, macostest.DeveloperIDInstaller)

	for _, tc := range []struct {
		name  string
		chain [][]byte
		kind  macos.CertificateKind
		warns bool
	}{
		{"application certificate for an application", [][]byte{application.Raw}, macos.DeveloperIDApplication, false},
		{"installer certificate for a package", [][]byte{installer.Raw}, macos.DeveloperIDInstaller, false},
		{"installer certificate for an application", [][]byte{installer.Raw}, macos.DeveloperIDApplication, true},
		{"application certificate for a package", [][]byte{application.Raw}, macos.DeveloperIDInstaller, true},
		{"other certificate", [][]byte{plain.Raw}, macos.DeveloperIDInstaller, true},
		{"only the leaf counts", [][]byte{plain.Raw, installer.Raw}, macos.DeveloperIDInstaller, true},
		// Nothing can be said about what cannot be read: ad hoc signers have
		// no chain at all.
		{"no chain", nil, macos.DeveloperIDInstaller, false},
		{"unreadable certificate", [][]byte{[]byte("not a certificate")}, macos.DeveloperIDInstaller, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := macostest.CaptureLogs(t)
			macos.WarnCertificateKind("pkg", "identity.p12", tc.chain, tc.kind)
			if !tc.warns {
				assert.Empty(t, logs.String())
				return
			}
			assert.Contains(t, logs.String(), "warning: pkg: the certificate in identity.p12 is not a "+tc.kind.Name+" certificate")
		})
	}
}
