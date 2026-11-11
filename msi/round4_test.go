package msi_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestMSIAllStringFieldsExpansion(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprintf("override=%v", override), func(t *testing.T) {
			icon, err := os.ReadFile("../testdata/msi/icon.dll")
			require.NoError(t, err)
			iconPath := filepath.Join(t.TempDir(), "icon$fixture.dll")
			require.NoError(t, os.WriteFile(iconPath, icon, 0o600))
			env := map[string]string{
				"ARCH": "x64", "VERSION": "2.3.4", "PRODUCT": "Expanded product",
				"MANUFACTURER": "Expanded manufacturer", "INSTALL_DIR": "Expanded",
				"PRODUCT_CODE": "{12345678-1234-1234-1234-123456789ABC}",
				"UPGRADE_CODE": "{ABCDEF01-2345-6789-ABCD-EF0123456789}",
				"MESSAGE":      "Cannot replace newer $$package", "ICON": iconPath,
				"SHORTCUT": "Expanded shortcut", "TARGET": "/Program Files/TestApp/app.exe",
				"DIRECTORY": "DesktopFolder", "DESCRIPTION": "Expanded description",
				"ARGUMENTS": "--expanded", "SERVICE": "ExpandedService", "DISPLAY": "Expanded service",
				"START_TYPE": "auto", "ACCOUNT": "NT AUTHORITY\\LocalService",
				"DEPENDENCY": "Tcpip", "ROOT": "HKLM", "KEY": "Software\\Expanded",
				"NAME": "ExpandedValue", "VALUE": "Expanded data", "FILE_VERSION": "3.4.5",
			}
			values := nfpm.MSI{
				Arch: "$ARCH", Version: "${VERSION}", ProductName: "$PRODUCT", Manufacturer: "$MANUFACTURER",
				ProductCode: "$PRODUCT_CODE", UpgradeCode: "$UPGRADE_CODE", InstallDir: "$INSTALL_DIR",
				Upgrade:      nfpm.MSIUpgrade{DowngradeErrorMessage: "$MESSAGE"},
				Properties:   map[string]string{"EXPANDED": "$VALUE"},
				FileVersions: map[string]string{env["TARGET"]: "$FILE_VERSION"},
				Shortcuts:    []nfpm.MSIShortcut{{Name: "$SHORTCUT", Target: "$TARGET", Directory: "$DIRECTORY", Arguments: "$ARGUMENTS", Description: "$DESCRIPTION", Icon: "$ICON"}},
				Services:     []nfpm.MSIService{{Name: "$SERVICE", DisplayName: "$DISPLAY", Executable: "$TARGET", Description: "$DESCRIPTION", StartType: "$START_TYPE", Account: "$ACCOUNT", Arguments: "$ARGUMENTS", Dependencies: []string{"$DEPENDENCY", "Dnscache"}}},
				Registry:     []nfpm.MSIRegistry{{Root: "$ROOT", Key: "$KEY", Name: "$NAME", Value: "$VALUE"}},
			}
			cfg := nfpm.Config{Info: *exampleInfo()}
			if override {
				cfg.Overrides = map[string]*nfpm.Overridables{"msi": {MSI: values}}
			} else {
				cfg.MSI = values
			}
			data, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			parsed, err := nfpm.ParseWithEnvMapping(bytes.NewReader(data), func(key string) string { return env[key] })
			require.NoError(t, err)
			info, err := parsed.Get("msi")
			require.NoError(t, err)
			db := buildDB(t, nfpm.WithDefaults(info))
			for name, want := range map[string]string{
				"ProductName": env["PRODUCT"], "Manufacturer": env["MANUFACTURER"],
				"ProductVersion": env["VERSION"], "ProductCode": env["PRODUCT_CODE"], "UpgradeCode": env["UPGRADE_CODE"], "EXPANDED": env["VALUE"],
			} {
				assert.Equal(t, want, property(t, db, name), name)
			}
			assert.Equal(t, "Expanded", longName(rowWhere(t, tableRows(t, db, "Directory"), "Directory", "INSTALLFOLDER")["DefaultDir"]))
			rowWhere(t, tableRows(t, db, "LaunchCondition"), "Description", env["MESSAGE"])
			assert.Equal(t, env["FILE_VERSION"], rowWhere(t, tableRows(t, db, "File"), "FileName", "app.exe")["Version"])
			shortcut := tableRows(t, db, "Shortcut")[0]
			assert.Equal(t, env["SHORTCUT"], longName(shortcut["Name"]))
			assert.Equal(t, env["DIRECTORY"], shortcut["Directory_"])
			assert.Equal(t, env["ARGUMENTS"], shortcut["Arguments"])
			assert.Equal(t, env["DESCRIPTION"], shortcut["Description"])
			assert.Equal(t, icon, tableRows(t, db, "Icon")[0]["Data"])
			service := tableRows(t, db, "ServiceInstall")[0]
			assert.Equal(t, env["SERVICE"], service["Name"])
			assert.Equal(t, env["DISPLAY"], service["DisplayName"])
			assert.Equal(t, env["DESCRIPTION"], service["Description"])
			assert.Equal(t, int32(2), service["StartType"])
			assert.Equal(t, env["ACCOUNT"], service["StartName"])
			assert.Equal(t, env["ARGUMENTS"], service["Arguments"])
			assert.Contains(t, service["Dependencies"], env["DEPENDENCY"])
			assert.Contains(t, service["Dependencies"], "Dnscache")
			registry := rowWhere(t, tableRows(t, db, "Registry"), "Name", env["NAME"])
			assert.Equal(t, int16(2), registry["Root"])
			assert.Equal(t, env["KEY"], registry["Key"])
			assert.Equal(t, env["VALUE"], registry["Value"])
		})
	}
}
