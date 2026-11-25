package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

const metadataConfig = `
name: foo
arch: amd64
version: v1.0.0
maintainer: Foo <foo@example.com>
description: Foo does things
enable_metadata: true
deb:
  metadata:
    distribution: unstable
    urgency: medium
`

func writeConfig(t *testing.T, config string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "nfpm.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	return path
}

func TestPackageMetadata(t *testing.T) {
	t.Run("directory containing .deb", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "out.deb")
		require.NoError(t, os.Mkdir(dir, 0o755))

		require.NoError(t, doPackage(writeConfig(t, metadataConfig), dir, "deb"))

		pkg, err := os.ReadFile(filepath.Join(dir, "foo_1.0.0_amd64.deb"))
		require.NoError(t, err)
		require.Greater(t, len(pkg), 100)
		require.Equal(t, "!<arch>\n", string(pkg[:8]))

		changes, err := os.ReadFile(filepath.Join(dir, "foo_1.0.0_amd64.changes"))
		require.NoError(t, err)
		require.Contains(t, string(changes), " "+strconv.Itoa(len(pkg))+" foo_1.0.0_amd64.deb\n")
	})

	t.Run("unsupported target keeps the package intact", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "artifact")

		err := doPackage(writeConfig(t, metadataConfig), target, "deb")
		require.ErrorContains(t, err, `use "foo_1.0.0_amd64.deb"`)

		pkg, err := os.ReadFile(target)
		require.NoError(t, err)
		require.Equal(t, "!<arch>\n", string(pkg[:8]))
		require.NoFileExists(t, target+".changes")
	})

	t.Run("missing metadata does not create a file", func(t *testing.T) {
		dir := t.TempDir()
		config := `
name: foo
arch: amd64
version: v1.0.0
enable_metadata: true
`
		err := doPackage(writeConfig(t, config), dir, "deb")
		require.ErrorContains(t, err, "deb.metadata.distribution is required")
		require.NoFileExists(t, filepath.Join(dir, "foo_1.0.0_amd64.changes"))
	})

	t.Run("disabled", func(t *testing.T) {
		dir := t.TempDir()
		config := `
name: foo
arch: amd64
version: v1.0.0
`
		require.NoError(t, doPackage(writeConfig(t, config), dir, "deb"))
		require.FileExists(t, filepath.Join(dir, "foo_1.0.0_amd64.deb"))
		require.NoFileExists(t, filepath.Join(dir, "foo_1.0.0_amd64.changes"))
	})
}
