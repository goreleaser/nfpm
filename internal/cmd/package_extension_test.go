package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionMismatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		target   string
		packager string
		ext      string
		other    string
	}{
		// Packager names are not extensions: srpm writes .src.rpm, which
		// filepath.Ext reports as .rpm.
		{"srpm with its own extension", "foo.src.rpm", "srpm", "", ""},
		{"rpm asked for a src.rpm", "foo.src.rpm", "rpm", ".src.rpm", "srpm"},
		// archlinux writes .pkg.tar.zst, which filepath.Ext reports as .zst.
		{"archlinux with its own extension", "foo.pkg.tar.zst", "archlinux", "", ""},
		{"deb asked for a pkg.tar.zst", "foo.pkg.tar.zst", "deb", ".pkg.tar.zst", "archlinux"},
		// The plain cases.
		{"matching", "foo.deb", "deb", "", ""},
		{"mismatched", "foo.deb", "rpm", ".deb", "deb"},
		{"in a directory", filepath.Join("dist", "foo.deb"), "deb", "", ""},
		{"relative to dot", "." + string(filepath.Separator) + "foo.deb", "deb", "", ""},
		{"no extension", "foo", "deb", "", ""},
		{"unknown extension", "foo.txt", "deb", "", ""},
		{"unknown packager", "foo.deb", "nope", ".deb", "deb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ext, other := extensionMismatch(tc.target, tc.packager)
			require.Equal(t, tc.ext, ext)
			require.Equal(t, tc.other, other)
		})
	}
}

// TestDoPackageRejectsMismatch covers the doPackage wiring: the check has to
// run before the config is read, so a nonexistent config still reports the
// extension mismatch.
func TestDoPackageRejectsMismatch(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nfpm.yaml")

	for _, tc := range []struct {
		name     string
		target   string
		packager string
		wantErr  string
	}{
		{"rpm asked for a src.rpm", "foo.src.rpm", "rpm", `target "foo.src.rpm" has the extension ".src.rpm" of the srpm packager, but "rpm" was requested`},
		{"deb asked for a pkg.tar.zst", "foo.pkg.tar.zst", "deb", `target "foo.pkg.tar.zst" has the extension ".pkg.tar.zst" of the archlinux packager, but "deb" was requested`},
		{"deb asked for a deb", "foo.deb", "deb", ""},
		{"srpm asked for a src.rpm", "foo.src.rpm", "srpm", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := doPackage(missing, filepath.Join(dir, tc.target), tc.packager)
			if tc.wantErr == "" {
				// No mismatch, so it gets as far as reading the config.
				require.Error(t, err)
				require.NotContains(t, err.Error(), "has the extension")
				return
			}
			require.EqualError(t, err, tc.wantErr)
			_, statErr := os.Stat(filepath.Join(dir, tc.target))
			require.True(t, os.IsNotExist(statErr), "target must not be created")
		})
	}
}
