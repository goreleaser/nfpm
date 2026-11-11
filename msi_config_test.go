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

func TestMSIOverrideEnvironment(t *testing.T) {
	config, err := nfpm.ParseWithEnvMapping(strings.NewReader(`
name: app
arch: amd64
version: 1.0.0
msi:
  manufacturer: Base
  file_versions:
    /app.exe: "1.0.0.0"
overrides:
  msi:
    msi:
      version: "$VERSION"
      product_name: "$NAME"
      manufacturer: "$VENDOR"
      install_dir: "$FOLDER"
      properties:
        EXAMPLE: "$PROPERTY"
      file_versions:
        /app.exe: ""
      shortcuts:
        - name: app
          target: /app.exe
          arguments: "$ARGUMENTS"
      services:
        - name: app
          executable: /app.exe
          account: "$ACCOUNT"
          arguments: "$ARGUMENTS"
      registry:
        - root: HKCU
          key: Software\app
          value: "$VALUE"
      signature:
        pfx_file: "$PFX"
        timestamp_url: "$TIMESTAMP"
`), func(key string) string {
		return map[string]string{
			"VERSION": "2.3.4", "NAME": "Example", "VENDOR": "Company", "FOLDER": "ExampleApp",
			"PROPERTY": "expanded", "ARGUMENTS": "--flag", "ACCOUNT": "LocalSystem", "VALUE": "registry data",
			"PFX": "cert.pfx", "TIMESTAMP": "https://timestamp.example.com", "NFPM_MSI_PASSPHRASE": "secret",
		}[key]
	})
	require.NoError(t, err)
	info, err := config.Get("msi")
	require.NoError(t, err)
	assert.Equal(t, "2.3.4", info.MSI.Version)
	assert.Equal(t, "Example", info.MSI.ProductName)
	assert.Equal(t, "Company", info.MSI.Manufacturer)
	assert.Equal(t, "ExampleApp", info.MSI.InstallDir)
	assert.Equal(t, "expanded", info.MSI.Properties["EXAMPLE"])
	assert.Empty(t, info.MSI.FileVersions["/app.exe"])
	assert.Equal(t, "--flag", info.MSI.Shortcuts[0].Arguments)
	assert.Equal(t, "LocalSystem", info.MSI.Services[0].Account)
	assert.Equal(t, "--flag", info.MSI.Services[0].Arguments)
	assert.Equal(t, "registry data", info.MSI.Registry[0].Value)
	assert.Equal(t, "cert.pfx", info.MSI.Signature.PFXFile)
	assert.Equal(t, "https://timestamp.example.com", info.MSI.Signature.TimestampURL)
	assert.Equal(t, "secret", info.MSI.Signature.KeyPassphrase)
}

func TestMSISchemaCaseVariants(t *testing.T) {
	data, err := os.ReadFile("www/static/schema.json")
	require.NoError(t, err)
	var schema struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Pattern string   `json:"pattern"`
				Enum    []string `json:"enum"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(data, &schema))
	for _, tc := range []struct {
		name, field    string
		valid, invalid []string
	}{
		{"MSIRegistry", "root", []string{"HKCU", "hkcu", "HkLm", "hkcr", "hkmU", "hku"}, []string{"HKCC", "HKLMx", "xHKCU"}},
		{"MSIService", "start_type", []string{"AUTO", "auto", "Demand", "DISABLED"}, []string{"boot", "system", "AUTOMATIC"}},
	} {
		p := schema.Defs[tc.name].Properties[tc.field]
		require.NotEmpty(t, p.Pattern)
		require.Empty(t, p.Enum, "a case-sensitive enum would still reject accepted variants")
		r, err := regexp.Compile(p.Pattern)
		require.NoError(t, err)
		for _, value := range tc.valid {
			assert.True(t, r.MatchString(value), value)
		}
		for _, value := range tc.invalid {
			assert.False(t, r.MatchString(value), value)
		}
	}
}
