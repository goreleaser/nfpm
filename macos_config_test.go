package nfpm_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func macOSEnv(key string) string {
	return map[string]string{
		"ARCH": "universal", "ID": "com.example.app", "MIN_OS": "13.0", "P12": "identity.p12",
		"TIMESTAMP": "https://timestamp.example.com", "NAME": "My App", "EXECUTABLE": "myapp",
		"ENTITLEMENTS": "app.entitlements", "URL": "myapp", "USAGE": "Needs the camera",
	}[key]
}

// pkgEnvironment sets every pkg setting from the environment. The schema
// must accept it as it is written.
const pkgEnvironment = `
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
`

func TestPkgEnvironment(t *testing.T) {
	config, err := nfpm.ParseWithEnvMapping(strings.NewReader(pkgEnvironment), macOSEnv)
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

// appEnvironment sets every app setting from the environment, indented for
// an override.
const appEnvironment = `
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

// appEnvironmentDocuments places appEnvironment in the base settings and in
// a dmg override.
func appEnvironmentDocuments() map[string]string {
	return map[string]string{
		"base": "name: app\narch: amd64\nversion: 1.0.0\napp:" +
			strings.ReplaceAll(appEnvironment, "\n      ", "\n  "),
		"override": "name: app\narch: amd64\nversion: 1.0.0\noverrides:\n  dmg:\n    app:" + appEnvironment,
	}
}

func TestAppEnvironment(t *testing.T) {
	for name, document := range appEnvironmentDocuments() {
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

// The process environment tells an unset variable from an empty one: an
// explicitly empty format passphrase selects an empty passphrase, which an
// identity exported without a password needs, even when NFPM_PASSPHRASE is
// set for other formats.
func TestMacOSPassphrasePrecedence(t *testing.T) {
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
	for _, variable := range []string{"NFPM_PKG_PASSPHRASE", "NFPM_APP_PASSPHRASE"} {
		for _, tc := range []struct {
			name, specific, want string
			set                  bool
		}{
			{"unset uses global", "", "global", false},
			{"specific overrides global", "specific", "specific", true},
			{"empty overrides global", "", "", true},
		} {
			t.Run(variable+" "+tc.name, func(t *testing.T) {
				t.Setenv("NFPM_PASSPHRASE", "global")
				t.Setenv(variable, tc.specific)
				if !tc.set {
					require.NoError(t, os.Unsetenv(variable))
				}
				passphrases := func(config nfpm.Config) map[string]string {
					pkg, err := config.Get("pkg")
					require.NoError(t, err)
					app, err := config.Get("app")
					require.NoError(t, err)
					return map[string]string{
						// The base settings and the overrides both carry it.
						"base pkg": config.Pkg.Signature.KeyPassphrase, "base app": config.App.Signature.KeyPassphrase,
						"NFPM_PKG_PASSPHRASE": pkg.Pkg.Signature.KeyPassphrase,
						"NFPM_APP_PASSPHRASE": app.App.Signature.KeyPassphrase,
					}
				}
				want := map[string]string{"base pkg": "global", "base app": "global", "NFPM_PKG_PASSPHRASE": "global", "NFPM_APP_PASSPHRASE": "global"}
				want[variable] = tc.want
				want["base "+strings.ToLower(strings.Split(variable, "_")[1])] = tc.want

				config, err := nfpm.Parse(strings.NewReader(document))
				require.NoError(t, err)
				assert.Equal(t, want, passphrases(config))
				path := filepath.Join(t.TempDir(), "nfpm.yaml")
				require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
				config, err = nfpm.ParseFile(path)
				require.NoError(t, err)
				assert.Equal(t, want, passphrases(config))
			})
		}
	}
}

// A custom mapping is the only source of variables: the process environment
// is not read, and an empty mapped value falls back to NFPM_PASSPHRASE.
func TestMacOSPassphraseCustomMappingIsolation(t *testing.T) {
	t.Setenv("NFPM_PKG_PASSPHRASE", "process-value")
	t.Setenv("NFPM_APP_PASSPHRASE", "process-value")
	mapping := func(key string) string {
		if key == "NFPM_PASSPHRASE" {
			return "mapped-global"
		}
		return ""
	}
	config, err := nfpm.ParseWithEnvMapping(strings.NewReader("name: app\n"), mapping)
	require.NoError(t, err)
	assert.Equal(t, "mapped-global", config.Pkg.Signature.KeyPassphrase)
	assert.Equal(t, "mapped-global", config.App.Signature.KeyPassphrase)
}

// Info.plist values nest dictionaries and arrays. An override for one format
// must not change the base settings, another format's settings or a result
// returned earlier, whatever order the formats are built in.
func TestInfoPlistOverridesAreIsolated(t *testing.T) {
	const document = `
name: app
arch: amd64
version: 1.0.0
app:
  identifier: com.example.app
  info_plist:
    NSAppTransportSecurity:
      NSAllowsArbitraryLoads: false
    CFBundleURLTypes:
      - CFBundleURLSchemes: [base]
overrides:
  dmg:
    app:
      info_plist:
        NSAppTransportSecurity:
          NSAllowsArbitraryLoads: true
        CFBundleURLTypes:
          - CFBundleURLSchemes: [dmg]
`
	base := map[string]any{
		"NSAppTransportSecurity": map[string]any{"NSAllowsArbitraryLoads": false},
		"CFBundleURLTypes":       []any{map[string]any{"CFBundleURLSchemes": []any{"base"}}},
	}
	dmg := map[string]any{
		"NSAppTransportSecurity": map[string]any{"NSAllowsArbitraryLoads": true},
		"CFBundleURLTypes":       []any{map[string]any{"CFBundleURLSchemes": []any{"dmg"}}},
	}
	for name, order := range map[string][]string{
		"app, dmg, app": {"app", "dmg", "app"},
		"dmg, app, dmg": {"dmg", "app", "dmg"},
	} {
		t.Run(name, func(t *testing.T) {
			config, err := nfpm.Parse(strings.NewReader(document))
			require.NoError(t, err)
			var results []*nfpm.Info
			for _, format := range order {
				info, err := config.Get(format)
				require.NoError(t, err)
				results = append(results, info)
				// Change the result too: it must not reach the configuration.
				info.App.InfoPlist["NSAppTransportSecurity"].(map[string]any)["Changed"] = format
			}
			for n, format := range order {
				delete(results[n].App.InfoPlist["NSAppTransportSecurity"].(map[string]any), "Changed")
				want := base
				if format == "dmg" {
					want = dmg
				}
				assert.Equal(t, want, results[n].App.InfoPlist, "result %d (%s)", n, format)
			}
			assert.Equal(t, base, config.App.InfoPlist, "the base settings")
		})
	}
}

// macOSSchema compiles the published schema with the validator the
// schema:validate task runs.
func macOSSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile("www/static/schema.json")
	require.NoError(t, err)
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	require.NoError(t, err)
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", document))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	return schema
}

// validateDocument checks a YAML configuration against the schema.
func validateDocument(t *testing.T, schema *jsonschema.Schema, document string) error {
	t.Helper()
	var decoded any
	require.NoError(t, yaml.Unmarshal([]byte(document), &decoded))
	data, err := json.Marshal(decoded)
	require.NoError(t, err)
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	require.NoError(t, err)
	return schema.Validate(instance)
}

// The schema must accept every configuration the packagers accept, so an
// editor never rejects a working one.
func TestMacOSSchemaAcceptsWorkingConfigurations(t *testing.T) {
	schema := macOSSchema(t)
	documents := map[string]string{
		"pkg settings from the environment": pkgEnvironment,
		// An override is one layer: the identifier comes from the base
		// settings, so the override need not repeat it.
		"partial overrides": `
name: app
arch: amd64
version: 1.0.0
app:
  identifier: com.example.app
pkg:
  identifier: com.example.app
overrides:
  dmg:
    app:
      name: My App
  pkg:
    pkg:
      signature:
        p12_file: installer.p12
`,
	}
	for name, document := range appEnvironmentDocuments() {
		documents["app settings from the environment, "+name] = document
	}
	for name, document := range documents {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, validateDocument(t, schema, document))
			// And the packagers accept them: the resolved values are valid.
			config, err := nfpm.ParseWithEnvMapping(strings.NewReader(document), macOSEnv)
			require.NoError(t, err)
			for _, format := range []string{"pkg", "app", "dmg"} {
				_, err := config.Get(format)
				require.NoError(t, err)
			}
		})
	}
}

func TestMacOSSchemaRejectsInvalidValues(t *testing.T) {
	schema := macOSSchema(t)
	for _, block := range []string{
		"pkg:\n  arch: amd64", "app:\n  arch: i386",
		"pkg:\n  identifier: .app", "app:\n  identifier: com.example.my_app",
		"pkg:\n  min_os_version: v11", "app:\n  min_os_version: 11.x",
		"overrides:\n  dmg:\n    app:\n      arch: aarch64",
	} {
		require.Error(t, validateDocument(t, schema, "name: app\nversion: 1.0.0\n"+block+"\n"), block)
	}
}

// The schema's patterns must match the packagers' own checks for literal
// values, and leave values from the environment to be checked once they are
// expanded.
func TestMacOSSchemaPatterns(t *testing.T) {
	data, err := os.ReadFile("www/static/schema.json")
	require.NoError(t, err)
	var schema struct {
		Defs map[string]struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Pattern string `json:"pattern"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(data, &schema))

	for _, name := range []string{"Pkg", "App"} {
		definition, ok := schema.Defs[name]
		require.True(t, ok, name)
		// The packagers require the identifier in the merged settings; any
		// single layer may leave it out.
		assert.Empty(t, definition.Required, name)
	}
	environment := []string{"$VALUE", "${VALUE}", "com.example.$NAME"}
	for _, tc := range []struct {
		name, field    string
		valid, invalid []string
	}{
		{"Pkg", "arch", []string{"arm64", "x86_64", "universal"}, []string{"", "amd64", "aarch64", "all", "arm64 "}},
		{"App", "arch", []string{"arm64", "x86_64", "universal"}, []string{"", "amd64", "aarch64", "all", "arm64 "}},
		{"Pkg", "identifier", []string{"com.example.app", "com.example.my_app", "app"}, []string{"", ".app", "com example", "com/example"}},
		{"App", "identifier", []string{"com.example.app", "com.example.my-app", "app"}, []string{"", ".app", "com.example.my_app", "com example"}},
		{"Pkg", "min_os_version", []string{"11", "11.0", "13.3.1"}, []string{"", "v11", "11.", "11.0.0.1", "11.x"}},
		{"App", "min_os_version", []string{"11", "11.0", "13.3.1"}, []string{"", "v11", "11.", "11.0.0.1", "11.x"}},
	} {
		pattern := schema.Defs[tc.name].Properties[tc.field].Pattern
		require.NotEmpty(t, pattern, "%s.%s", tc.name, tc.field)
		compiled, err := regexp.Compile(pattern)
		require.NoError(t, err)
		for _, value := range append(tc.valid, environment...) {
			assert.True(t, compiled.MatchString(value), "%s.%s should accept %q", tc.name, tc.field, value)
		}
		for _, value := range tc.invalid {
			assert.False(t, compiled.MatchString(value), "%s.%s should reject %q", tc.name, tc.field, value)
		}
	}
}
