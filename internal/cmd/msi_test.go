package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	goversion "github.com/caarlos0/go-version"
	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/internal/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestMSIReleaseArtifactsDoNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	payload := filepath.Join(t.TempDir(), "payload")
	require.NoError(t, os.WriteFile(payload, []byte("payload"), 0o600))
	configFile := filepath.Join(t.TempDir(), "nfpm.yaml")
	config := nfpm.Config{Info: nfpm.Info{
		Name: "artifact", Arch: "amd64", Vendor: "Vendor",
		Overridables: nfpm.Overridables{Contents: files.Contents{{Source: payload, Destination: "/app.bin"}}},
	}}
	var firstPath string
	var firstBytes []byte
	for _, version := range []string{"1.2.3-rc1", "1.2.3"} {
		config.Version = version
		data, err := yaml.Marshal(config)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(configFile, data, 0o600))
		cmd.Execute(goversion.Info{}, func(code int) { assert.Zero(t, code) },
			[]string{"pkg", "--config", configFile, "--packager", "msi", "--target", dir})
		matches, err := filepath.Glob(filepath.Join(dir, "*.msi"))
		require.NoError(t, err)
		if firstPath == "" {
			require.Len(t, matches, 1)
			firstPath = matches[0]
			firstBytes, err = os.ReadFile(firstPath)
			require.NoError(t, err)
		} else {
			require.Len(t, matches, 2)
			after, err := os.ReadFile(firstPath)
			require.NoError(t, err)
			assert.Equal(t, firstBytes, after)
		}
	}
}
