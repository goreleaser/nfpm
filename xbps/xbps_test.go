package xbps

import (
	"bytes"
	"errors"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/stretchr/testify/require"
)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:            "foo",
		Arch:            "amd64",
		Version:         "v1.2.3",
		Prerelease:      "beta-1",
		VersionMetadata: "git-1",
		Release:         "2",
	})
}

func TestRegistered(t *testing.T) {
	packager, err := nfpm.Get(packagerName)
	require.NoError(t, err)
	require.Equal(t, Default, packager)
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".xbps", Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	info := exampleInfo()
	require.Equal(t, "foo-1.2.3.beta-1.git-1_2.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameDefaultRelease(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	require.Equal(t, "foo-1.2.3.beta-1.git-1_1.x86_64.xbps", Default.ConventionalFileName(info))
}

func TestConventionalFileNameNoarch(t *testing.T) {
	info := exampleInfo()
	info.Arch = "all"
	require.Equal(t, "foo-1.2.3.beta-1.git-1_2.noarch.xbps", Default.ConventionalFileName(info))
}

func TestEnsureValidArchOverride(t *testing.T) {
	info := exampleInfo()
	info.XBPS.Arch = "ppc64le"
	normalized, err := ensureValidArch(info)
	require.NoError(t, err)
	require.Equal(t, "ppc64le", normalized.Arch)
}

func TestEnsureValidArchMappings(t *testing.T) {
	testCases := map[string]string{
		"all":     "noarch",
		"noarch":  "noarch",
		"amd64":   "x86_64",
		"x86_64":  "x86_64",
		"386":     "i686",
		"i386":    "i686",
		"i686":    "i686",
		"arm64":   "aarch64",
		"aarch64": "aarch64",
		"arm6":    "armv6l",
		"arm7":    "armv7l",
	}

	for input, expected := range testCases {
		input, expected := input, expected
		t.Run(input, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = input
			normalized, err := ensureValidArch(info)
			require.NoError(t, err)
			require.Equal(t, expected, normalized.Arch)
		})
	}
}

func TestEnsureValidArchUnknown(t *testing.T) {
	info := exampleInfo()
	info.Arch = "loong64"
	_, err := ensureValidArch(info)
	require.ErrorContains(t, err, `unsupported architecture "loong64"`)
}

func TestVersionNormalizesLeadingVAndParts(t *testing.T) {
	info := exampleInfo()
	require.Equal(t, "1.2.3.beta-1.git-1", version(info))

	info.Prerelease = "-rc1-"
	info.VersionMetadata = ".build2."
	require.Equal(t, "1.2.3.rc1.build2", version(info))
}

func TestRevisionDefaultsToOneWhenEmpty(t *testing.T) {
	info := exampleInfo()
	info.Release = ""
	rev, err := revision(info)
	require.NoError(t, err)
	require.Equal(t, "1", rev)
}

func TestRevisionRejectsNonPositiveInteger(t *testing.T) {
	for _, release := range []string{"beta1", "0", "-1"} {
		release := release
		t.Run(release, func(t *testing.T) {
			info := exampleInfo()
			info.Release = release
			_, err := revision(info)
			require.ErrorContains(t, err, "must be a positive integer revision")
		})
	}
}

func TestPackageRejectsNonLinux(t *testing.T) {
	info := exampleInfo()
	info.Platform = "windows"

	err := Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, "invalid platform")
}

func TestPackageRejectsUnknownArch(t *testing.T) {
	info := exampleInfo()
	info.Arch = "loong64"

	err := Default.Package(info, &bytes.Buffer{})
	require.ErrorContains(t, err, `unsupported architecture "loong64"`)
}

func TestPackagePlaceholder(t *testing.T) {
	err := Default.Package(exampleInfo(), &bytes.Buffer{})
	require.True(t, errors.Is(err, errPackageWriterNotImplemented))
}
