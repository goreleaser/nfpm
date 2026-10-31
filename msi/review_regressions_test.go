package msi_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/msi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewIdentity(t *testing.T) {
	t.Run("architecture aliases", func(t *testing.T) {
		a, b := exampleInfo(), exampleInfo()
		a.MSI.Arch, b.MSI.Arch = "intel", "X86"
		x, y := buildDB(t, a), buildDB(t, b)
		assert.Equal(t, property(t, x, "ProductCode"), property(t, y, "ProductCode"))
		assert.Equal(t, property(t, x, "UpgradeCode"), property(t, y, "UpgradeCode"))
		assert.Equal(t, componentGUIDs(t, x), componentGUIDs(t, y))
	})
	t.Run("override retains shared release", func(t *testing.T) {
		a, b := exampleInfo(), exampleInfo()
		a.Version, b.Version = "2024.1.0", "2024.1.0"
		a.MSI.Version, b.MSI.Version = "24.1.0", "24.1.0"
		a.Prerelease = "rc1"
		x, y := buildDB(t, a), buildDB(t, b)
		assert.NotEqual(t, property(t, x, "ProductCode"), property(t, y, "ProductCode"))
		assert.Equal(t, property(t, x, "UpgradeCode"), property(t, y, "UpgradeCode"))
		assert.Equal(t, componentGUIDs(t, x), componentGUIDs(t, y))
	})
	t.Run("unambiguous product tuples", func(t *testing.T) {
		a, b := exampleInfo(), exampleInfo()
		a.MSI.Manufacturer, a.MSI.ProductName = "Acme|Suite", "Tool"
		b.MSI.Manufacturer, b.MSI.ProductName = "Acme", "Suite|Tool"
		a.MSI.InstallDir, b.MSI.InstallDir = "TestApp", "TestApp"
		x, y := buildDB(t, a), buildDB(t, b)
		assert.NotEqual(t, property(t, x, "ProductCode"), property(t, y, "ProductCode"))
		assert.NotEqual(t, property(t, x, "UpgradeCode"), property(t, y, "UpgradeCode"))
	})
	t.Run("unambiguous registry tuples", func(t *testing.T) {
		i := exampleInfo()
		i.MSI.Registry = []nfpm.MSIRegistry{
			{Root: "HKLM", Key: `Software\Acme|Sub`, Name: "Value", Value: "one"},
			{Root: "HKLM", Key: `Software\Acme`, Name: "Sub|Value", Value: "two"},
		}
		db := buildDB(t, i)
		r := tableRows(t, db, "Registry")
		require.Len(t, r, 2)
		assert.NotEqual(t, r[0]["Component_"], r[1]["Component_"])
	})
}

func TestReviewEqualVersionPolicy(t *testing.T) {
	for _, downgrade := range []bool{false, true} {
		i := exampleInfo()
		i.MSI.Upgrade.AllowDowngrades = downgrade
		db := buildDB(t, i)
		r := rowWhere(t, tableRows(t, db, "Upgrade"), "ActionProperty", "NFPM_EQUAL_PRODUCTS")
		assert.Equal(t, "1.2.3", r["VersionMin"])
		assert.Equal(t, "1.2.3", r["VersionMax"])
		assert.Equal(t, int32(0x302), r["Attributes"])
		assert.Contains(t, property(t, db, "SecureCustomProperties"), "NFPM_EQUAL_PRODUCTS")
		rowWhere(t, tableRows(t, db, "LaunchCondition"), "Condition", "Installed OR NOT NFPM_EQUAL_PRODUCTS")
	}
}

func TestReviewMalformedVersions(t *testing.T) {
	for _, v := range []string{"1.x.3", "999999999999999999999999999999.2.3", "-1.2.3", "1.-2.3", "1.2.-3", "1..3", "1.2.3.4.5"} {
		t.Run(v, func(t *testing.T) {
			i := exampleInfo()
			i.MSI.Version = v
			var out bytes.Buffer
			err := msi.Default.Package(i, &out)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "msi.version")
			assert.Zero(t, out.Len())
		})
	}
	for version, expected := range map[string]string{"1": "1.0.0", "1.2": "1.2.0", "v1.2.3-rc1+meta": "1.2.3"} {
		i := exampleInfo()
		i.MSI.Version = version
		assert.Equal(t, expected, property(t, buildDB(t, i), "ProductVersion"))
	}
}

func TestReviewDirectoryAliases(t *testing.T) {
	i := exampleInfo()
	i.Contents[0].Destination = "/App/a.exe"
	i.Contents[1].Destination = "/app/b.conf"
	db := buildDB(t, i)
	var count int
	for _, row := range tableRows(t, db, "Directory") {
		if strings.EqualFold(row["DefaultDir"].(string), "App") {
			count++
		}
	}
	assert.Equal(t, 1, count, "case aliases share one directory")
	assert.Len(t, tableRows(t, db, "File"), 2)
}

func TestReviewResourceCollisions(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*nfpm.Info)
	}{
		{"file ancestor", func(i *nfpm.Info) { i.Contents[0].Destination = "/data"; i.Contents[1].Destination = "/DATA/sub.txt" }},
		{"resolved ancestor", func(i *nfpm.Info) {
			i.Contents[0].Destination = "/data"
			i.Contents[1].Destination = "/Program Files/TestApp/data/sub.txt"
		}},
		{"explicit directory", func(i *nfpm.Info) {
			i.Contents = append(i.Contents, &files.Content{Type: files.TypeDir, Destination: "/PROGRAM FILES/TESTAPP/APP.EXE"})
		}},
		{"registry case", func(i *nfpm.Info) {
			i.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKLM", Key: `Software\Audit`, Name: "Value", Value: "one"}, {Root: "HKLM", Key: `software\AUDIT`, Name: "value", Value: "two"}}
		}},
		{"registry context", func(i *nfpm.Info) {
			i.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKLM", Key: `Software\Audit`, Name: "Value", Value: "one"}, {Root: "HKMU", Key: `Software\Audit`, Name: "Value", Value: "two"}}
		}},
		{"registry classes", func(i *nfpm.Info) {
			i.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKLM", Key: `Software\Classes\Audit`, Name: "Value", Value: "one"}, {Root: "HKCR", Key: "Audit", Name: "Value", Value: "two"}}
		}},
		{"shortcut case", func(i *nfpm.Info) {
			i.MSI.Shortcuts = []nfpm.MSIShortcut{{Name: "Audit Tool", Target: i.Contents[0].Destination, Directory: "DesktopFolder"}, {Name: "audit tool", Target: i.Contents[1].Destination, Directory: "DesktopFolder"}}
		}},
		{"service case", func(i *nfpm.Info) {
			i.MSI.Services = []nfpm.MSIService{{Name: "AuditService", Executable: i.Contents[0].Destination}, {Name: "auditservice", Executable: i.Contents[1].Destination}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := exampleInfo()
			tc.edit(i)
			var out bytes.Buffer
			err := msi.Default.Package(i, &out)
			require.Error(t, err)
			assert.Zero(t, out.Len())
		})
	}
}

func TestReviewEmptyDirectories(t *testing.T) {
	for _, tree := range []bool{false, true} {
		i := exampleInfo()
		c := &files.Content{Type: files.TypeDir, Destination: "/empty"}
		if tree {
			c.Source = t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(c.Source, "child"), 0o700))
			c.Type = files.TypeTree
		}
		i.Contents = append(i.Contents, c)
		var out bytes.Buffer
		err := msi.Default.Package(i, &out)
		require.ErrorContains(t, err, "empty director")
		assert.Zero(t, out.Len())
	}
}

func TestReviewLicenseSelection(t *testing.T) {
	i := exampleInfo()
	i.MSI.MinimalUI = true
	for _, name := range []string{"z-first", "a-second"} {
		p := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.WriteFile(p, []byte(name), 0o600))
		i.Contents = append(i.Contents, &files.Content{Source: p, Destination: "/" + name, Type: files.TypeRPMLicense})
	}
	db := buildDB(t, i)
	r := rowWhere(t, tableRows(t, db, "Control"), "Control", "License")
	assert.Equal(t, "z-first", r["Text"])
}

func TestReviewPerUserShortcuts(t *testing.T) {
	for _, dir := range []string{"WindowsFolder", "WindowsVolume", "SystemFolder", "System64Folder", "FontsFolder", "CommonAppDataFolder"} {
		t.Run(dir, func(t *testing.T) {
			i := exampleInfo()
			i.MSI.PerUser = true
			i.Contents = i.Contents[1:]
			i.MSI.Shortcuts = []nfpm.MSIShortcut{{Name: "Audit", Target: i.Contents[0].Destination, Directory: dir}}
			var out bytes.Buffer
			require.ErrorContains(t, msi.Default.Package(i, &out), "per-user")
			assert.Zero(t, out.Len())
		})
	}
}

func TestReviewPowerShellEncoding(t *testing.T) {
	for _, content := range [][]byte{[]byte("Write-Output 'Café 日本語'"), append([]byte{0xef, 0xbb, 0xbf}, []byte("Write-Output 'Café'")...), {0xff, 0xfe, 'e', 0, 'x', 0, 'i', 0, 't', 0, ' ', 0, '0', 0}} {
		i := exampleInfo()
		p := filepath.Join(t.TempDir(), "hook.ps1")
		require.NoError(t, os.WriteFile(p, content, 0o600))
		i.Scripts.PostInstall = p
		db := buildDB(t, i)
		ca, _ := customAction(t, db, "NfpmPostInstall")
		target := ca["Target"].(string)
		encoded, err := base64.StdEncoding.DecodeString(target[strings.LastIndexByte(target, ' ')+1:])
		require.NoError(t, err)
		words := make([]uint16, len(encoded)/2)
		for n := range words {
			words[n] = binary.LittleEndian.Uint16(encoded[n*2:])
		}
		bootstrap := string(utf16.Decode(words))
		expected := content
		if !bytes.HasPrefix(content, []byte{0xff, 0xfe}) && !bytes.HasPrefix(content, []byte{0xef, 0xbb, 0xbf}) {
			expected = append([]byte{0xef, 0xbb, 0xbf}, content...)
		}
		assert.Contains(t, bootstrap, base64.StdEncoding.EncodeToString(expected))
	}
}

func TestReleasedDependencyRegressions(t *testing.T) {
	t.Run("complete package identity", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "payload")
		i := exampleInfo()
		i.Contents = files.Contents{{Source: p, Destination: "/app.exe"}}
		require.NoError(t, os.WriteFile(p, []byte("PAYLOAD-A"), 0o600))
		baseline := packageAndValidate(t, i)
		assert.Equal(t, baseline, packageAndValidate(t, i), "same input is reproducible")
		code := openDB(t, baseline).Summary().Revision()
		require.NoError(t, os.WriteFile(p, []byte("PAYLOAD-B"), 0o600))
		assert.NotEqual(t, code, buildDB(t, i).Summary().Revision())
		require.NoError(t, os.WriteFile(p, []byte("PAYLOAD-A"), 0o600))
		i.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKLM", Key: `Software\Audit`, Name: "Value", Value: "one"}}
		assert.NotEqual(t, code, buildDB(t, i).Summary().Revision())
		i.MSI.Registry = nil
		script := filepath.Join(t.TempDir(), "hook.ps1")
		require.NoError(t, os.WriteFile(script, []byte("exit 0"), 0o600))
		i.Scripts.PostInstall = script
		assert.NotEqual(t, code, buildDB(t, i).Summary().Revision())
	})
	t.Run("literal short name reservation", func(t *testing.T) {
		i := exampleInfo()
		i.Contents[0].Destination = "/abcdef1long.txt"
		i.Contents[1].Destination = "/abcdef~1.txt"
		i.MSI.Properties = map[string]string{"SHORTFILENAMES": "1"}
		rows := tableRows(t, buildDB(t, i), "File")
		require.Len(t, rows, 2)
		a := strings.ToUpper(strings.Split(rows[0]["FileName"].(string), "|")[0])
		b := strings.ToUpper(strings.Split(rows[1]["FileName"].(string), "|")[0])
		assert.NotEqual(t, a, b)
	})
	t.Run("modeless progress", func(t *testing.T) {
		i := exampleInfo()
		i.MSI.MinimalUI = true
		r := rowWhere(t, tableRows(t, buildDB(t, i), "Dialog"), "Dialog", "ProgressDlg")
		assert.Zero(t, r["Attributes"].(int32)&2)
	})
	for _, name := range []string{"NUL.txt", "CON", "COM1.log", "COM¹.txt", "readme.txt.", "readme.txt "} {
		t.Run(name, func(t *testing.T) {
			i := exampleInfo()
			i.Contents[0].Destination = "/" + name
			var out bytes.Buffer
			require.Error(t, msi.Default.Package(i, &out))
			assert.Zero(t, out.Len())
		})
	}
}
