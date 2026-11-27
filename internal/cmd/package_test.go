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

// guessConfig writes a configuration every packager can build: one script,
// installed as a command and, for the app packager, as the main executable
// of the bundle.
func guessConfig(t *testing.T) string {
	t.Helper()
	program := filepath.Join(t.TempDir(), "guess")
	require.NoError(t, os.WriteFile(program, []byte("#!/bin/sh\necho guess\n"), 0o600))
	content := func(destination string) files.Contents {
		return files.Contents{{
			Source: program, Destination: destination,
			FileInfo: &files.ContentFileInfo{Mode: 0o755},
		}}
	}
	config := nfpm.Config{
		Info: nfpm.Info{
			Name: "guess", Arch: "arm64", Version: "1.0.0", Maintainer: "Guess <guess@example.com>",
			Description: "guess", Vendor: "Vendor",
			Overridables: nfpm.Overridables{
				Contents: content("/usr/local/bin/guess"),
				App:      nfpm.App{Identifier: "com.example.guess"},
				Pkg:      nfpm.Pkg{Identifier: "com.example.guess"},
			},
		},
		Overrides: map[string]*nfpm.Overridables{
			"app": {Contents: content("/Contents/MacOS/guess")},
		},
	}
	data, err := yaml.Marshal(config)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "nfpm.yaml")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// Without --packager the packager is guessed from the target. A target with
// a two-part extension, such as .app.zip, names its packager by the whole
// extension.
func TestPackagerIsGuessedFromTheTarget(t *testing.T) {
	config := guessConfig(t)
	for target, magic := range map[string]string{
		"guess.app.zip":     "PK\x03\x04",
		"guess.pkg.tar.zst": "\x28\xb5\x2f\xfd",
		"guess.pkg":         "xar!",
		"guess.deb":         "!<arch>\n",
	} {
		t.Run(target, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), target)
			cmd.Execute(goversion.Info{}, func(code int) { assert.Zero(t, code) },
				[]string{"pkg", "--config", config, "--target", path})
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Greater(t, len(data), len(magic))
			assert.Equal(t, magic, string(data[:len(magic)]), "%s is built by the right packager", target)
		})
	}
}

func TestUnknownTargetExtension(t *testing.T) {
	var code int
	cmd.Execute(goversion.Info{}, func(c int) { code = c },
		[]string{"pkg", "--config", guessConfig(t), "--target", filepath.Join(t.TempDir(), "guess.zip")})
	assert.NotZero(t, code)
}
