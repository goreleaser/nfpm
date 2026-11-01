package msi_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/msi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestRound3InstallLocation(t *testing.T) {
	for _, perUser := range []bool{false, true} {
		i := exampleInfo()
		i.MSI.PerUser = perUser
		i.Contents = i.Contents[1:]
		db := buildDB(t, i)
		saved := rowWhere(t, tableRows(t, db, "Registry"), "Name", "InstallFolder")
		root := int16(2)
		if perUser {
			root = 1
		}
		assert.Equal(t, root, saved["Root"])
		assert.Equal(t, "[INSTALLFOLDER]", saved["Value"])
		locator := rowWhere(t, tableRows(t, db, "RegLocator"), "Key", saved["Key"])
		assert.Equal(t, root, locator["Root"])
		assert.Equal(t, int16(2), locator["Type"], "metadata is explicitly in the 32-bit registry view")
		component := rowWhere(t, tableRows(t, db, "Component"), "Component", saved["Component_"])
		assert.Equal(t, attrRegistryKeyPath, component["Attributes"])
		assert.Equal(t, saved["Registry"], component["KeyPath"])
		assert.Contains(t, property(t, db, "SecureCustomProperties"), "INSTALLFOLDER")
		assert.Contains(t, property(t, db, "SecureCustomProperties"), "NFPM_SAVED_INSTALLFOLDER")
		ca := rowWhere(t, tableRows(t, db, "CustomAction"), "Action", "NfpmRestoreInstallFolder")
		assert.Equal(t, int16(51), ca["Type"])
		assert.Equal(t, "INSTALLFOLDER", ca["Source"])
		assert.Equal(t, "[NFPM_SAVED_INSTALLFOLDER]", ca["Target"])
		for _, table := range []string{"InstallExecuteSequence", "InstallUISequence"} {
			rows := tableRows(t, db, table)
			restore := rowWhere(t, rows, "Action", "NfpmRestoreInstallFolder")
			assert.Equal(t, "NOT INSTALLFOLDER AND NFPM_SAVED_INSTALLFOLDER", restore["Condition"])
			assert.Less(t, rowWhere(t, rows, "Action", "AppSearch")["Sequence"].(int16), restore["Sequence"].(int16))
			assert.Less(t, restore["Sequence"].(int16), rowWhere(t, rows, "Action", "CostInitialize")["Sequence"].(int16))
		}
	}
}

func TestRound3MSIExpansion(t *testing.T) {
	for _, override := range []bool{false, true} {
		i := exampleInfo()
		baseline := buildDB(t, i)
		component := tableRows(t, baseline, "Component")[0]["Component"].(string)
		reference := "[$" + component + "]"
		cfg := nfpm.Config{Info: *exampleInfo()}
		values := nfpm.MSI{Properties: map[string]string{"AMOUNT": "Cost $$5", "BUILD": "${BUILD_VALUE}", "LITERAL": "$$literal", "COMPONENT_DIRECTORY": reference}, Registry: []nfpm.MSIRegistry{{Root: "HKLM", Key: "Software\\Expansion", Name: "Value", Value: reference + " - $$5 - $BUILD_VALUE"}}}
		if override {
			cfg.Overrides = map[string]*nfpm.Overridables{"msi": {MSI: values}}
		} else {
			cfg.MSI = values
		}
		data, err := yaml.Marshal(cfg)
		require.NoError(t, err)
		parsed, err := nfpm.ParseWithEnvMapping(bytes.NewReader(data), func(key string) string {
			if key == "BUILD_VALUE" {
				return "expanded"
			}
			return ""
		})
		require.NoError(t, err)
		info, err := parsed.Get("msi")
		require.NoError(t, err)
		db := buildDB(t, nfpm.WithDefaults(info))
		assert.Equal(t, "Cost $5", property(t, db, "AMOUNT"))
		assert.Equal(t, "expanded", property(t, db, "BUILD"))
		assert.Equal(t, "$literal", property(t, db, "LITERAL"))
		assert.Equal(t, reference, property(t, db, "COMPONENT_DIRECTORY"))
		row := rowWhere(t, tableRows(t, db, "Registry"), "Name", "Value")
		assert.Equal(t, reference+" - $5 - expanded", row["Value"])
	}
}

func TestRound3SymlinkPayloads(t *testing.T) {
	for _, tree := range []bool{false, true} {
		i := exampleInfo()
		dir := t.TempDir()
		source := filepath.Join(t.TempDir(), "payload")
		require.NoError(t, os.WriteFile(source, []byte("executable"), 0o600))
		link := filepath.Join(dir, "app.exe")
		if err := os.Symlink(source, link); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("symlink privilege unavailable: %v", err)
			}
			require.NoError(t, err)
		}
		c := &files.Content{Source: link, Destination: "/app.exe"}
		if tree {
			c.Source, c.Type, c.Destination = dir, files.TypeTree, "/tree"
		}
		i.Contents = append(i.Contents, c)
		var out bytes.Buffer
		require.ErrorContains(t, msi.Default.Package(i, &out), "symlink")
		assert.Zero(t, out.Len())
	}
}

func TestRound3MissingOKWarnings(t *testing.T) {
	for _, tree := range []bool{false, true} {
		t.Run(fmt.Sprint(tree), func(t *testing.T) {
			logs := captureLogs(t)
			i := exampleInfo()
			c := i.Contents[1]
			c.Type = files.TypeConfigMissingOK
			if tree {
				c.Source = t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(c.Source, "app.conf"), []byte("data"), 0o600))
				c.Destination, c.Type = "/etc", files.TypeConfigMissingOKTree
			}
			buildDB(t, i)
			assert.Contains(t, logs.String(), "installed as a regular file and replaced on upgrade")
		})
	}
}

func TestRound3DisabledService(t *testing.T) {
	i := exampleInfo()
	i.MSI.Services = []nfpm.MSIService{{Name: "Disabled", Executable: i.Contents[0].Destination, StartType: "DISABLED", Start: true}}
	var out bytes.Buffer
	require.ErrorContains(t, msi.Default.Package(i, &out), "disabled")
	assert.Zero(t, out.Len())
	i.MSI.Services[0].Start = false
	db := buildDB(t, i)
	assert.Equal(t, int32(4), tableRows(t, db, "ServiceInstall")[0]["StartType"])
	assert.Zero(t, tableRows(t, db, "ServiceControl")[0]["Event"].(int16)&serviceEventStart)
}

func TestRound3FilenameIdentity(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range []struct{ version, release, override string }{
		{"1.2.3-rc1", "", ""},
		{"1.2.3", "", ""},
		{"1.2.3+build.1", "", ""},
		{"1.2.3+build.2", "", ""},
		{"2024.1.0-rc1", "", "24.1.0"},
		{"2024.1.0", "", "24.1.0"},
		{"1.2.3", "2", ""},
	} {
		i := nfpm.WithDefaults(&nfpm.Info{Name: "app", Version: tc.version, Release: tc.release, Arch: "amd64"})
		i.MSI.Version = tc.override
		name := msi.Default.ConventionalFileName(i)
		assert.False(t, seen[name], "distinct release fields collided: %s", name)
		seen[name] = true
		assert.NotContains(t, strings.TrimSuffix(name, ".msi"), "/")
	}
}
