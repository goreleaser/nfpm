package nfpm_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func macOSEnv(key string) string {
	return map[string]string{
		"ARCH": "universal", "ID": "com.example.app", "MIN_OS": "13.0", "P12": "identity.p12",
		"TIMESTAMP": "https://timestamp.example.com", "NAME": "My App", "EXECUTABLE": "myapp",
		"ENTITLEMENTS": "app.entitlements", "URL": "myapp", "USAGE": "Needs the camera",
	}[key]
}

func TestPkgEnvironment(t *testing.T) {
	config, err := nfpm.ParseWithEnvMapping(strings.NewReader(`
name: app
arch: amd64
version: 1.0.0
pkg:
  arch: "$ARCH"
  identifier: "$ID"
  min_os_version: "$MIN_OS"
  signature:
    p12_file: "$P12"
    timestamp_url: "$TIMESTAMP"
`), macOSEnv)
	require.NoError(t, err)
	info, err := config.Get("pkg")
	require.NoError(t, err)
	assert.Equal(t, "universal", info.Pkg.Arch)
	assert.Equal(t, "com.example.app", info.Pkg.Identifier)
	assert.Equal(t, "13.0", info.Pkg.MinOSVersion)
	assert.Equal(t, "identity.p12", info.Pkg.Signature.P12File)
	assert.Equal(t, "https://timestamp.example.com", info.Pkg.Signature.TimestampURL)
	assert.Empty(t, info.Pkg.Signature.KeyPassphrase)
}

// Overrides are merged after parsing, so they need the same expansion as the
// base settings.
func TestPkgOverrideEnvironment(t *testing.T) {
	config, err := nfpm.ParseWithEnvMapping(strings.NewReader(`
name: app
arch: amd64
version: 1.0.0
pkg:
  identifier: com.example.base
overrides:
  pkg:
    pkg:
      arch: "$ARCH"
      identifier: "$ID"
      min_os_version: "$MIN_OS"
      signature:
        p12_file: "$P12"
        timestamp_url: "$TIMESTAMP"
`), macOSEnv)
	require.NoError(t, err)
	info, err := config.Get("pkg")
	require.NoError(t, err)
	assert.Equal(t, "universal", info.Pkg.Arch)
	assert.Equal(t, "com.example.app", info.Pkg.Identifier)
	assert.Equal(t, "13.0", info.Pkg.MinOSVersion)
	assert.Equal(t, "identity.p12", info.Pkg.Signature.P12File)
	assert.Equal(t, "https://timestamp.example.com", info.Pkg.Signature.TimestampURL)

	other, err := config.Get("deb")
	require.NoError(t, err)
	assert.Equal(t, "com.example.base", other.Pkg.Identifier, "an override applies to its own packager only")
}

func TestAppEnvironment(t *testing.T) {
	const settings = `
      arch: "$ARCH"
      name: "$NAME"
      identifier: "$ID"
      executable: "$EXECUTABLE"
      min_os_version: "$MIN_OS"
      info_plist:
        NSCameraUsageDescription: "$USAGE"
        LSUIElement: true
        CFBundleURLTypes:
          - CFBundleURLSchemes: ["$URL"]
      signature:
        p12_file: "$P12"
        timestamp_url: "$TIMESTAMP"
        entitlements: "$ENTITLEMENTS"
`
	for name, document := range map[string]string{
		"base": "name: app\narch: amd64\nversion: 1.0.0\napp:" +
			strings.ReplaceAll(settings, "\n      ", "\n  "),
		"override": "name: app\narch: amd64\nversion: 1.0.0\noverrides:\n  dmg:\n    app:" + settings,
	} {
		t.Run(name, func(t *testing.T) {
			config, err := nfpm.ParseWithEnvMapping(strings.NewReader(document), macOSEnv)
			require.NoError(t, err)
			info, err := config.Get("dmg")
			require.NoError(t, err)
			assert.Equal(t, "universal", info.App.Arch)
			assert.Equal(t, "My App", info.App.Name)
			assert.Equal(t, "com.example.app", info.App.Identifier)
			assert.Equal(t, "myapp", info.App.Executable)
			assert.Equal(t, "13.0", info.App.MinOSVersion)
			assert.Equal(t, "Needs the camera", info.App.InfoPlist["NSCameraUsageDescription"])
			assert.Equal(t, true, info.App.InfoPlist["LSUIElement"])
			assert.Equal(t,
				[]any{map[string]any{"CFBundleURLSchemes": []any{"myapp"}}},
				info.App.InfoPlist["CFBundleURLTypes"],
				"strings are expanded however deeply they are nested")
			assert.Equal(t, "identity.p12", info.App.Signature.P12File)
			assert.Equal(t, "https://timestamp.example.com", info.App.Signature.TimestampURL)
			assert.Equal(t, "app.entitlements", info.App.Signature.Entitlements)
		})
	}
}

func TestMacOSPassphrases(t *testing.T) {
	const document = `
name: app
arch: amd64
version: 1.0.0
overrides:
  pkg:
    pkg:
      identifier: com.example.app
  app:
    app:
      identifier: com.example.app
`
	for _, tc := range []struct {
		name     string
		env      map[string]string
		pkg, app string
	}{
		{"none", map[string]string{}, "", ""},
		{"general", map[string]string{"NFPM_PASSPHRASE": "general"}, "general", "general"},
		{"specific", map[string]string{
			"NFPM_PASSPHRASE": "general", "NFPM_PKG_PASSPHRASE": "installer", "NFPM_APP_PASSPHRASE": "application",
		}, "installer", "application"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := nfpm.ParseWithEnvMapping(strings.NewReader(document), func(key string) string {
				return tc.env[key]
			})
			require.NoError(t, err)
			// The base settings and the override both carry the passphrase.
			assert.Equal(t, tc.pkg, config.Pkg.Signature.KeyPassphrase)
			assert.Equal(t, tc.app, config.App.Signature.KeyPassphrase)
			info, err := config.Get("pkg")
			require.NoError(t, err)
			assert.Equal(t, tc.pkg, info.Pkg.Signature.KeyPassphrase)
			info, err = config.Get("app")
			require.NoError(t, err)
			assert.Equal(t, tc.app, info.App.Signature.KeyPassphrase)
		})
	}
}

func TestMacOSUnknownFieldsAreRejected(t *testing.T) {
	for _, block := range []string{"pkg:\n  bundle_id: com.example.app", "app:\n  bundle: com.example.app"} {
		_, err := nfpm.Parse(strings.NewReader("name: app\narch: amd64\nversion: 1.0.0\n" + block + "\n"))
		require.Error(t, err, block)
	}
}

// The schema must accept exactly what the packagers accept, so an editor
// neither rejects a working configuration nor approves a broken one.
func TestMacOSSchemaMatchesThePackagers(t *testing.T) {
	data, err := os.ReadFile("www/static/schema.json")
	require.NoError(t, err)
	var schema struct {
		Defs map[string]struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Pattern string   `json:"pattern"`
				Enum    []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(data, &schema))

	for _, name := range []string{"Pkg", "App"} {
		definition, ok := schema.Defs[name]
		require.True(t, ok, name)
		assert.Equal(t, []string{"identifier"}, definition.Required, name)
		assert.Equal(t, []string{"arm64", "x86_64", "universal"}, definition.Properties["arch"].Enum, name)
	}
	for _, tc := range []struct {
		name, field    string
		valid, invalid []string
	}{
		{"Pkg", "identifier", []string{"com.example.app", "com.example.my_app", "app"}, []string{"", ".app", "com example", "com/example"}},
		{"App", "identifier", []string{"com.example.app", "com.example.my-app", "app"}, []string{"", ".app", "com.example.my_app", "com example"}},
		{"Pkg", "min_os_version", []string{"11", "11.0", "13.3.1"}, []string{"", "v11", "11.", "11.0.0.1", "11.x"}},
		{"App", "min_os_version", []string{"11", "11.0", "13.3.1"}, []string{"", "v11", "11.", "11.0.0.1", "11.x"}},
	} {
		pattern := schema.Defs[tc.name].Properties[tc.field].Pattern
		require.NotEmpty(t, pattern, "%s.%s", tc.name, tc.field)
		compiled, err := regexp.Compile(pattern)
		require.NoError(t, err)
		for _, value := range tc.valid {
			assert.True(t, compiled.MatchString(value), "%s.%s should accept %q", tc.name, tc.field, value)
		}
		for _, value := range tc.invalid {
			assert.False(t, compiled.MatchString(value), "%s.%s should reject %q", tc.name, tc.field, value)
		}
	}
}
