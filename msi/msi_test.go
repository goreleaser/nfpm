package msi_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/goreleaser/nfpm/v2/msi"
	gomsi "go.digitalxero.dev/go-msi"
	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cfbMagic is the OLE Compound File header shared by .msi files.
// nolint: gochecknoglobals
var cfbMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// Component.Attributes bits (Microsoft Learn).
const (
	attrRegistryKeyPath int16 = 0x4
	attrPermanent       int16 = 0x10
	attr64bit           int16 = 0x100
)

// Upgrade.Attributes bits and ServiceControl.Event bits (Microsoft Learn).
const (
	upgradeMigrateFeatures     int32 = 0x1
	upgradeOnlyDetect          int32 = 0x2
	upgradeVersionMaxInclusive int32 = 0x200

	serviceEventStart           int16 = 0x1
	serviceEventStop            int16 = 0x2
	serviceEventUninstallStop   int16 = 0x20
	serviceEventUninstallDelete int16 = 0x80
)

// CustomAction.Type bits: type 34 is an executable run from a directory; the
// modifiers make it deferred, run as SYSTEM, and hide its command line from
// the log.
const (
	caTypeMask      int16 = 0x3F
	caTypeEXEInDir  int16 = 34
	caDeferred      int16 = 0x400
	caNoImpersonate int16 = 0x800
	caHideTarget    int16 = 0x2000
)

// Golden values of the derivation contract: the upgrade code and component
// GUIDs of packages already installed on users' machines are derived from
// these seeds, so a change here silently orphans every existing install.
// Update them only with a deliberate, documented migration.
const (
	goldenUpgradeCode   = "{98EF4BAC-CB49-52FF-A5CB-A37D3B48CD6E}"
	goldenComponentGUID = "{E1A61890-5690-5C92-B173-3466FCA5EB93}"
	goldenRegistryGUID  = "{C6308198-2D0C-5B62-9813-92795F87F50F}"
)

func exampleInfo() *nfpm.Info {
	return nfpm.WithDefaults(&nfpm.Info{
		Name:        "TestApp",
		Arch:        "amd64",
		Description: "Test application",
		Version:     "v1.2.3",
		Maintainer:  "Test <test@example.com>",
		Vendor:      "TestCo",
		Homepage:    "https://example.com",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{
					Source:      "../testdata/fake",
					Destination: "/Program Files/TestApp/app.exe",
				},
				{
					Source:      "../testdata/whatever.conf",
					Destination: "/app/config.conf",
				},
			},
			MSI: nfpm.MSI{
				Manufacturer: "Test Company",
			},
		},
	})
}

// packageAndValidate builds an MSI, asserts it is a structurally valid,
// ICE-clean Windows Installer database, and returns the raw package bytes.
func packageAndValidate(t *testing.T, info *nfpm.Info) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, msi.Default.Package(info, &buf))
	require.Positive(t, buf.Len(), "package should not be empty")
	require.True(t, bytes.HasPrefix(buf.Bytes(), cfbMagic), "output should be a CFB container")

	v, err := gomsi.NewValidator().WithAllICEs().Build()
	require.NoError(t, err)
	findings, err := v.Validate(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	for _, f := range findings {
		if f.Severity() == gomsi.SeverityError {
			t.Errorf("ICE error finding %s: %s", f.ICE(), f.Message())
		}
	}
	return buf.Bytes()
}

// buildDB packages info and opens the result for table assertions.
func buildDB(t *testing.T, info *nfpm.Info) gomsi.Database {
	t.Helper()
	return openDB(t, packageAndValidate(t, info))
}

func openDB(t *testing.T, raw []byte) gomsi.Database {
	t.Helper()
	db, err := gomsi.Open(bytes.NewReader(raw))
	require.NoError(t, err)
	return db
}

// tableRows returns the rows of a table, failing the test if it is absent.
func tableRows(t *testing.T, db gomsi.Database, table string) []gomsi.Row {
	t.Helper()
	rows, err := db.Table(table)
	require.NoError(t, err, "table %s", table)
	return rows
}

// hasTable reports whether the package lists the table at all.
func hasTable(db gomsi.Database, table string) bool {
	_, err := db.Table(table)
	return err == nil
}

// rowWhere returns the first row whose column has the given value, failing
// the test when none does.
func rowWhere(t *testing.T, rows []gomsi.Row, column string, value any) gomsi.Row {
	t.Helper()
	for _, r := range rows {
		if r[column] == value {
			return r
		}
	}
	require.Failf(t, "row not found", "no row with %s == %v", column, value)
	return nil
}

// property returns the value of a Property row, or "" when absent.
func property(t *testing.T, db gomsi.Database, name string) string {
	t.Helper()
	for _, r := range tableRows(t, db, "Property") {
		if r["Property"] == name {
			v, _ := r["Value"].(string)
			return v
		}
	}
	return ""
}

// componentGUIDs maps every Component ID to its ComponentId GUID.
func componentGUIDs(t *testing.T, db gomsi.Database) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range tableRows(t, db, "Component") {
		out[r["Component"].(string)], _ = r["ComponentId"].(string)
	}
	return out
}

// componentFor returns the Component row owning the file with the given long
// name.
func componentFor(t *testing.T, db gomsi.Database, fileName string) gomsi.Row {
	t.Helper()
	for _, f := range tableRows(t, db, "File") {
		name, _ := f["FileName"].(string)
		if name == fileName || strings.HasSuffix(name, "|"+fileName) {
			return rowWhere(t, tableRows(t, db, "Component"), "Component", f["Component_"])
		}
	}
	require.Failf(t, "file not found", "no File row named %q", fileName)
	return nil
}

// longName strips the short half of a short|long Filename cell.
func longName(v any) string {
	s, _ := v.(string)
	if _, long, ok := strings.Cut(s, "|"); ok {
		return long
	}
	return s
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &logs
}

func TestConventionalExtension(t *testing.T) {
	require.Equal(t, ".msi", msi.Default.ConventionalExtension())
}

func TestConventionalFileName(t *testing.T) {
	// Exercises both arch mapping (amd64 -> x64) and version conversion
	// (v1.2.3 -> 1.2.3).
	require.Equal(t, "TestApp_1.2.3_x64.msi", msi.Default.ConventionalFileName(exampleInfo()))
}

func TestArchMapping(t *testing.T) {
	tests := map[string]string{
		"amd64":   "x64",
		"x86_64":  "x64",
		"386":     "x86",
		"i386":    "x86",
		"arm64":   "arm64",
		"aarch64": "arm64",
		"arm":     "arm",
		"arm7":    "arm",
		"ia64":    "intel64",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = in
			name := msi.Default.ConventionalFileName(info)
			require.Contains(t, name, "_"+want+".msi")

			var buf bytes.Buffer
			require.NoError(t, msi.Default.Package(info, &buf),
				"every mapped architecture must be packageable")
		})
	}
}

// TestArchSetsPlatform proves the architecture reaches the SummaryInformation
// Template, so a package declares the platform it was actually built for
// instead of always claiming x64.
func TestArchSetsPlatform(t *testing.T) {
	for arch, template := range map[string]string{
		"amd64":   "x64;",
		"386":     "Intel;",
		"arm":     "Arm;",
		"arm64":   "Arm64;",
		"ia64":    "Intel64;",
		"x86_64":  "x64;",
		"aarch64": "Arm64;",
	} {
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch

			db := buildDB(t, info)
			require.True(t, strings.HasPrefix(db.Summary().Template(), template),
				"expected Template platform %q for arch %q, got %q", template, arch, db.Summary().Template())
		})
	}
}

// TestUnsupportedArch proves an architecture Windows Installer has no platform
// for is rejected rather than silently packaged as a 32-bit x86 install.
func TestUnsupportedArch(t *testing.T) {
	for _, arch := range []string{"all", "neutral", "ppc64le", "s390x", "riscv64", "mips", "totally-bogus"} {
		t.Run(arch, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = arch

			var buf bytes.Buffer
			err := msi.Default.Package(info, &buf)
			require.Error(t, err)
			require.Contains(t, err.Error(), "is not supported by Windows Installer")
		})
	}
}

func TestArchOverride(t *testing.T) {
	info := exampleInfo()
	info.Arch = "amd64"
	info.MSI.Arch = "x86"
	require.Contains(t, msi.Default.ConventionalFileName(info), "_x86.msi")

	// Case must not create a different product line.
	upper := exampleInfo()
	upper.MSI.Arch = "X64"
	require.Equal(t, "TestApp_1.2.3_x64.msi", msi.Default.ConventionalFileName(upper))
	require.Equal(t, property(t, buildDB(t, exampleInfo()), "UpgradeCode"), property(t, buildDB(t, upper), "UpgradeCode"))
}

func TestVersionConversion(t *testing.T) {
	tests := map[string]string{
		"1.2.3":                  "1.2.3",
		"v1.2.3":                 "1.2.3",
		"1.0.0":                  "1.0.0",
		"2.5":                    "2.5.0",
		"1":                      "1.0.0",
		"1.2.3.4":                "1.2.3",
		"v1.0.0-0.1.b1+git.abcd": "1.0.0",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			info := exampleInfo()
			info.Version = in
			require.Equal(t, "TestApp_"+want+"_x64.msi", msi.Default.ConventionalFileName(info))
		})
	}
}

// TestVersionOutOfRange proves a version Windows Installer cannot represent is
// an error rather than a silently clamped (and therefore colliding) one.
func TestVersionOutOfRange(t *testing.T) {
	t.Run("shared version", func(t *testing.T) {
		info := exampleInfo()
		info.Version = "2024.1.0"
		err := msi.Default.Package(info, io.Discard)
		require.Error(t, err)
		require.Contains(t, err.Error(), `version "2024.1.0"`)
		require.Contains(t, err.Error(), "set msi.version")
	})

	t.Run("msi.version", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.Version = "1.2.70000"
		err := msi.Default.Package(info, io.Discard)
		require.Error(t, err)
		require.Contains(t, err.Error(), `msi.version "1.2.70000"`)
	})
}

// TestVersionOverride proves msi.version replaces the shared version in both the
// package and its conventional file name.
func TestVersionOverride(t *testing.T) {
	info := exampleInfo()
	info.Version = "2024.1.0"
	info.MSI.Version = "24.1.0"

	require.Equal(t, "TestApp_24.1.0_x64.msi", msi.Default.ConventionalFileName(info))
	require.Equal(t, "24.1.0", property(t, buildDB(t, info), "ProductVersion"))
}

func TestPackageMinimal(t *testing.T) {
	db := buildDB(t, exampleInfo())
	require.Equal(t, "TestApp", property(t, db, "ProductName"))
	require.Equal(t, "1.2.3", property(t, db, "ProductVersion"))
	require.Equal(t, "Test Company", property(t, db, "Manufacturer"))
	require.Equal(t, "1", property(t, db, "ALLUSERS"), "per-machine by default")
}

func TestPackageWithContents(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents,
		&files.Content{Source: "../testdata/whatever.conf", Destination: "/Program Files/TestApp/sub/extra.txt"},
		&files.Content{Source: "../testdata/whatever.conf", Destination: "relative/path/file.txt"},
	)
	db := buildDB(t, info)

	var names []string
	for _, f := range tableRows(t, db, "File") {
		names = append(names, longName(f["FileName"]))
	}
	require.ElementsMatch(t, []string{"app.exe", "config.conf", "extra.txt", "file.txt"}, names)
}

// TestInstallFolder proves where the product's own files land: under the
// platform's Program Files folder for per-machine installs, with the
// configured install directory name.
func TestInstallFolder(t *testing.T) {
	parentOf := func(t *testing.T, db gomsi.Database, dir string) (string, string) {
		t.Helper()
		row := rowWhere(t, tableRows(t, db, "Directory"), "Directory", dir)
		parent, _ := row["Directory_Parent"].(string)
		return parent, longName(row["DefaultDir"])
	}

	t.Run("64bit", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.InstallDir = "My Application"
		db := buildDB(t, info)
		parent, name := parentOf(t, db, "INSTALLFOLDER")
		require.Equal(t, "ProgramFiles64Folder", parent)
		require.Equal(t, "My Application", name)

		// Contents without a well-known prefix live under it.
		comp := componentFor(t, db, "config.conf")
		dir := rowWhere(t, tableRows(t, db, "Directory"), "Directory", comp["Directory_"])
		require.Equal(t, "INSTALLFOLDER", dir["Directory_Parent"])
	})

	t.Run("32bit", func(t *testing.T) {
		info := exampleInfo()
		info.Arch = "386"
		db := buildDB(t, info)
		parent, name := parentOf(t, db, "INSTALLFOLDER")
		require.Equal(t, "ProgramFilesFolder", parent)
		require.Equal(t, "TestApp", name, "install_dir defaults to the product name")
		require.False(t, hasTable(db, "ProgramFiles64Folder"))
		for _, r := range tableRows(t, db, "Directory") {
			require.NotEqual(t, "ProgramFiles64Folder", r["Directory"],
				"a 32-bit package must not reference the 64-bit program files folder")
		}
	})

	t.Run("per user", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.PerUser = true
		info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: "/app.exe"}}
		db := buildDB(t, info)

		parent, _ := parentOf(t, db, "INSTALLFOLDER")
		require.Equal(t, "LocalProgramsFolder", parent)
		parent, name := parentOf(t, db, "LocalProgramsFolder")
		require.Equal(t, "LocalAppDataFolder", parent)
		require.Equal(t, "Programs", name)
		parent, _ = parentOf(t, db, "LocalAppDataFolder")
		require.Equal(t, "TARGETDIR", parent)
		require.Empty(t, property(t, db, "ALLUSERS"), "a per-user package must not set ALLUSERS")
	})
}

// TestPerUserValidation proves a per-user package cannot claim per-machine
// resources it could not create without elevation.
func TestPerUserValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*nfpm.Info)
		expect string
	}{
		{
			name:   "program files destination",
			mutate: func(*nfpm.Info) {},
			expect: "per-machine location",
		},
		{
			name: "system32 destination",
			mutate: func(info *nfpm.Info) {
				info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: "/Windows/System32/x.dll"}}
			},
			expect: "per-machine location",
		},
		{
			name: "HKLM registry",
			mutate: func(info *nfpm.Info) {
				info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: "/app.exe"}}
				info.MSI.Registry = []nfpm.MSIRegistry{{Root: "hklm", Key: `Software\X`, Name: "a", Value: "b"}}
			},
			expect: "HKLM requires a per-machine install",
		},
		{
			name: "service",
			mutate: func(info *nfpm.Info) {
				info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: "/svc.exe"}}
				info.MSI.Services = []nfpm.MSIService{{Name: "Svc", Executable: "/svc.exe"}}
			},
			expect: "Windows services require a per-machine install",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := exampleInfo()
			info.MSI.PerUser = true
			tt.mutate(info)
			err := msi.Default.Package(info, io.Discard)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expect)
		})
	}

	t.Run("HKCU registry is fine", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.PerUser = true
		info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: "/app.exe"}}
		info.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKCU", Key: `Software\X`, Name: "a", Value: "b"}}
		buildDB(t, info)
	})
}

// TestManufacturerFallback proves an MSI builds without any msi-specific
// manufacturer: the root vendor is embedded instead.
func TestManufacturerFallback(t *testing.T) {
	info := exampleInfo()
	info.MSI.Manufacturer = ""
	require.Equal(t, "TestCo", property(t, buildDB(t, info), "Manufacturer"))
}

func TestNoManufacturer(t *testing.T) {
	info := exampleInfo()
	info.MSI.Manufacturer = ""
	info.Vendor = ""
	info.Maintainer = ""
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msi.manufacturer, vendor, or maintainer")
}

func TestManufacturerDefaults(t *testing.T) {
	tests := []struct {
		name         string
		vendor       string
		maintainer   string
		manufacturer string
		want         string
	}{
		{"explicit wins", "TestCo", "Jane Doe <jane@example.com>", "Explicit Co", "Explicit Co"},
		{"vendor", "TestCo", "Jane Doe <jane@example.com>", "", "TestCo"},
		{"maintainer email stripped", "", "Jane Doe <jane@example.com>", "", "Jane Doe"},
		{"maintainer without email", "", "Jane Doe", "", "Jane Doe"},
		{"all empty", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := exampleInfo()
			info.Vendor = tt.vendor
			info.Maintainer = tt.maintainer
			info.MSI.Manufacturer = tt.manufacturer
			msi.Default.SetPackagerDefaults(info)
			require.Equal(t, tt.want, info.MSI.Manufacturer)
		})
	}
}

func TestInvalidProductCode(t *testing.T) {
	for _, code := range []string{
		"not-a-guid",                              // no braces
		"{not-a-guid}",                            // braced but not hex {8-4-4-4-12}
		"{12345678-1234-1234-1234-123456789AB}",   // node too short
		"{12345678-1234-1234-1234-123456789ABCD}", // node too long
		"{12345678-1234-1234-1234-123456789ABG}",  // non-hex digit
		"12345678-1234-1234-1234-123456789ABC",    // missing braces
	} {
		t.Run(code, func(t *testing.T) {
			info := exampleInfo()
			info.MSI.ProductCode = code
			var buf bytes.Buffer
			err := msi.Default.Package(info, &buf)
			require.Error(t, err)
			require.Contains(t, err.Error(), "product_code")
		})
	}
}

func TestExplicitGUIDs(t *testing.T) {
	info := exampleInfo()
	info.MSI.ProductCode = "{12345678-1234-1234-1234-123456789ABC}"
	info.MSI.UpgradeCode = "{ABCDEF01-2345-6789-ABCD-EF0123456789}"
	db := buildDB(t, info)
	require.Equal(t, "{12345678-1234-1234-1234-123456789ABC}", property(t, db, "ProductCode"))
	require.Equal(t, "{ABCDEF01-2345-6789-ABCD-EF0123456789}", property(t, db, "UpgradeCode"))
}

// TestLowercaseGUIDs proves that a canonical but lowercase GUID is accepted:
// validation is case-insensitive and the codes are normalized to uppercase
// before go-msi (which requires uppercase) sees them.
func TestLowercaseGUIDs(t *testing.T) {
	info := exampleInfo()
	info.MSI.ProductCode = "{12345678-1234-1234-1234-123456789abc}"
	info.MSI.UpgradeCode = "{abcdef01-2345-6789-abcd-ef0123456789}"
	db := buildDB(t, info)
	require.Equal(t, "{12345678-1234-1234-1234-123456789ABC}", property(t, db, "ProductCode"))
	require.Equal(t, "{ABCDEF01-2345-6789-ABCD-EF0123456789}", property(t, db, "UpgradeCode"))
}

// TestDerivedCodes guards against shipping an MSI without a ProductCode
// (msiexec fails such installs with error 1605) and pins the derivation
// contract: the codes of packages already installed on users' machines are
// derived from these seeds, so they must never change by accident.
func TestDerivedCodes(t *testing.T) {
	info := exampleInfo()
	require.Empty(t, info.MSI.ProductCode)
	require.Empty(t, info.MSI.UpgradeCode)

	var buf bytes.Buffer
	require.NoError(t, msi.Default.Package(info, &buf))
	db := openDB(t, buf.Bytes())

	require.Equal(t, goldenUpgradeCode, property(t, db, "UpgradeCode"))
	require.NotEmpty(t, property(t, db, "ProductCode"))
	require.NotEqual(t, property(t, db, "ProductCode"), property(t, db, "UpgradeCode"))

	comp := componentFor(t, db, "app.exe")
	require.Equal(t, goldenComponentGUID, comp["ComponentId"])

	// Derivation must be reproducible.
	var buf2 bytes.Buffer
	require.NoError(t, msi.Default.Package(exampleInfo(), &buf2))
	require.Equal(t, buf.Bytes(), buf2.Bytes(), "builds with identical input must be reproducible")
}

// TestComponentGUIDsStableAcrossReleases is the Windows Installer component
// rule: the same resource at the same path keeps its component GUID in every
// release. Without it, uninstalling an older release removes the files of a
// newer one.
func TestComponentGUIDsStableAcrossReleases(t *testing.T) {
	v1 := exampleInfo()
	v1.Version = "1.0.0"
	v2 := exampleInfo()
	v2.Version = "1.1.0"
	v2.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKLM", Key: `Software\TestCo\TestApp`, Name: "InstallPath", Value: "x"}}
	v1.MSI.Registry = v2.MSI.Registry

	db1, db2 := buildDB(t, v1), buildDB(t, v2)
	require.NotEqual(t, property(t, db1, "ProductCode"), property(t, db2, "ProductCode"),
		"every release needs its own ProductCode")
	require.Equal(t, property(t, db1, "UpgradeCode"), property(t, db2, "UpgradeCode"))
	require.Equal(t, componentGUIDs(t, db1), componentGUIDs(t, db2),
		"component GUIDs must not change between releases")
}

// TestComponentGUIDsDifferPerArch proves x86 and x64 builds sharing a pinned
// upgrade code still get distinct component GUIDs: they install different
// resources (different files, different registry views).
func TestComponentGUIDsDifferPerArch(t *testing.T) {
	x64 := exampleInfo()
	x64.MSI.UpgradeCode = "{ABCDEF01-2345-6789-ABCD-EF0123456789}"
	x86 := exampleInfo()
	x86.Arch = "386"
	x86.MSI.UpgradeCode = x64.MSI.UpgradeCode

	g64, g86 := componentGUIDs(t, buildDB(t, x64)), componentGUIDs(t, buildDB(t, x86))
	require.Len(t, g86, len(g64))
	for id, guid := range g64 {
		require.NotEqual(t, guid, g86[id], "component %s must not share a GUID across architectures", id)
	}
}

func TestShortcut(t *testing.T) {
	info := exampleInfo()
	info.MSI.Shortcuts = []nfpm.MSIShortcut{
		{
			Name:        "Test App",
			Target:      "/Program Files/TestApp/app.exe",
			Description: "Launch Test App",
		},
	}
	db := buildDB(t, info)

	rows := tableRows(t, db, "Shortcut")
	require.Len(t, rows, 1)
	require.Equal(t, "Test App", longName(rows[0]["Name"]))
	require.Contains(t, rows[0]["Name"], "|", "a name with a space needs the short|long form")
	require.Equal(t, "ProgramMenuFolder", rows[0]["Directory_"], "defaults to the Start menu")
	require.Equal(t, "Launch Test App", rows[0]["Description"])
	require.Equal(t, "MainFeature", rows[0]["Target"], "advertised shortcuts target the feature")
	require.Equal(t, componentFor(t, db, "app.exe")["Component"], rows[0]["Component_"])
}

func TestShortcutTargetNotInContents(t *testing.T) {
	info := exampleInfo()
	info.MSI.Shortcuts = []nfpm.MSIShortcut{
		{Name: "Test App", Target: "/Program Files/TestApp/missing.exe"},
	}
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match any contents destination")
}

func TestShortcutInvalidDirectory(t *testing.T) {
	info := exampleInfo()
	info.MSI.Shortcuts = []nfpm.MSIShortcut{
		{Name: "Test App", Target: "/Program Files/TestApp/app.exe", Directory: "NotARealFolder"},
	}
	err := msi.Default.Package(info, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), `msi.shortcuts[0].directory "NotARealFolder"`)
}

// TestShortcutOptionalFields exercises the optional shortcut fields, including
// an icon, which adds an Icon row to the package.
func TestShortcutOptionalFields(t *testing.T) {
	info := exampleInfo()
	info.MSI.Shortcuts = []nfpm.MSIShortcut{
		{
			Name:        "Test App",
			Target:      "/Program Files/TestApp/app.exe",
			Directory:   "DesktopFolder",
			Description: "Launch Test App",
			Arguments:   "--verbose",
			Icon:        "../testdata/fake",
		},
	}
	db := buildDB(t, info)
	rows := tableRows(t, db, "Shortcut")
	require.Len(t, rows, 1)
	require.Equal(t, "DesktopFolder", rows[0]["Directory_"])
	require.Equal(t, "--verbose", rows[0]["Arguments"])
	icons := tableRows(t, db, "Icon")
	require.Len(t, icons, 1)
	require.Equal(t, icons[0]["Name"], rows[0]["Icon_"])
}

func TestShortcutMissingIcon(t *testing.T) {
	info := exampleInfo()
	info.MSI.Shortcuts = []nfpm.MSIShortcut{
		{
			Name:   "Test App",
			Target: "/Program Files/TestApp/app.exe",
			Icon:   "/does/not/exist.ico",
		},
	}
	err := msi.Default.Package(info, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "shortcut icon")
}

func withService(svc nfpm.MSIService) *nfpm.Info {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/Program Files/TestApp/svc.exe",
	})
	svc.Executable = "/Program Files/TestApp/svc.exe"
	info.MSI.Services = []nfpm.MSIService{svc}
	return info
}

// TestService proves a service is registered on the executable's component and
// controlled on both install and uninstall: stopped before its files are
// touched, started when asked, and stopped and deleted on removal.
func TestService(t *testing.T) {
	db := buildDB(t, withService(nfpm.MSIService{
		Name:         "TestSvc",
		DisplayName:  "Test Service",
		Description:  "A test service",
		StartType:    "auto",
		Account:      "NT AUTHORITY\\LocalService",
		Arguments:    "--serve",
		Dependencies: []string{"Tcpip", "Dnscache"},
		Start:        true,
	}))

	comp := componentFor(t, db, "svc.exe")
	installs := tableRows(t, db, "ServiceInstall")
	require.Len(t, installs, 1)
	si := installs[0]
	require.Equal(t, "TestSvc", si["Name"])
	require.Equal(t, "Test Service", si["DisplayName"])
	require.Equal(t, "A test service", si["Description"])
	require.Equal(t, int32(2), si["StartType"], "auto start")
	require.Equal(t, "NT AUTHORITY\\LocalService", si["StartName"])
	require.Equal(t, "--serve", si["Arguments"])
	require.Contains(t, si["Dependencies"], "Tcpip")
	require.Equal(t, comp["Component"], si["Component_"])

	controls := tableRows(t, db, "ServiceControl")
	require.Len(t, controls, 1)
	events := controls[0]["Event"].(int16)
	require.Equal(t, serviceEventStop|serviceEventStart|serviceEventUninstallStop|serviceEventUninstallDelete, events)
	require.Equal(t, comp["Component"], controls[0]["Component_"])
}

// TestServiceNoStart proves the service is still stopped on install and
// stopped and deleted on uninstall when it should not be started.
func TestServiceNoStart(t *testing.T) {
	db := buildDB(t, withService(nfpm.MSIService{Name: "TestSvc", StartType: "demand"}))
	controls := tableRows(t, db, "ServiceControl")
	require.Len(t, controls, 1)
	events := controls[0]["Event"].(int16)
	require.Equal(t, serviceEventStop|serviceEventUninstallStop|serviceEventUninstallDelete, events)
	require.Equal(t, int32(3), tableRows(t, db, "ServiceInstall")[0]["StartType"], "demand start")
}

func TestServiceTargetNotInContents(t *testing.T) {
	info := exampleInfo()
	info.MSI.Services = []nfpm.MSIService{
		{Name: "TestSvc", Executable: "/Program Files/TestApp/missing.exe", StartType: "demand"},
	}
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match any contents destination")
}

func TestServiceInvalidStartType(t *testing.T) {
	// boot and system are driver-only start types; the ServiceInstall table
	// always emits Win32 own-process services, for which CreateService rejects
	// them, so they must not be accepted.
	for _, startType := range []string{"bogus", "boot", "system"} {
		t.Run(startType, func(t *testing.T) {
			info := exampleInfo()
			info.MSI.Services = []nfpm.MSIService{
				{Name: "TestSvc", Executable: "/Program Files/TestApp/app.exe", StartType: startType},
			}
			var buf bytes.Buffer
			err := msi.Default.Package(info, &buf)
			require.Error(t, err)
			require.Contains(t, err.Error(), "start_type")
		})
	}
}

func TestServiceValidStartTypes(t *testing.T) {
	for startType, want := range map[string]int32{"auto": 2, "demand": 3, "disabled": 4, "AUTO": 2} {
		t.Run(startType, func(t *testing.T) {
			info := exampleInfo()
			info.MSI.Services = []nfpm.MSIService{
				{Name: "TestSvc", Executable: "/Program Files/TestApp/app.exe", StartType: startType},
			}
			db := buildDB(t, info)
			require.Equal(t, want, tableRows(t, db, "ServiceInstall")[0]["StartType"])
		})
	}
}

// TestRegistry proves registry values are written from their own components,
// keyed by the value (with the RegistryKeyPath attribute Windows Installer
// needs to read the KeyPath as a Registry row rather than a File row), with a
// GUID that survives releases.
func TestRegistry(t *testing.T) {
	info := exampleInfo()
	info.MSI.Registry = []nfpm.MSIRegistry{
		{Root: "HKLM", Key: `Software\TestCo\TestApp`, Name: "InstallPath", Value: "C:\\TestApp"},
		{Root: "hkcu", Key: `Software\TestCo\TestApp`, Name: "Enabled", Value: "1"},
	}
	db := buildDB(t, info)

	regs := tableRows(t, db, "Registry")
	require.Len(t, regs, 2)
	hklm := rowWhere(t, regs, "Name", "InstallPath")
	require.Equal(t, int16(2), hklm["Root"], "HKLM")
	require.Equal(t, `Software\TestCo\TestApp`, hklm["Key"])
	require.Equal(t, "C:\\TestApp", hklm["Value"])
	hkcu := rowWhere(t, regs, "Name", "Enabled")
	require.Equal(t, int16(1), hkcu["Root"], "HKCU, case-insensitively")

	comps := tableRows(t, db, "Component")
	for _, reg := range regs {
		comp := rowWhere(t, comps, "Component", reg["Component_"])
		require.Equal(t, reg["Registry"], comp["KeyPath"], "the value is the component's key path")
		attrs := comp["Attributes"].(int16)
		require.NotZero(t, attrs&attrRegistryKeyPath, "KeyPath must be flagged as a registry key path")
		require.NotZero(t, attrs&attr64bit, "registry rows of a 64-bit package bypass WOW6432Node")
		require.Equal(t, "INSTALLFOLDER", comp["Directory_"])
	}
	require.Equal(t, goldenRegistryGUID, rowWhere(t, comps, "Component", hklm["Component_"])["ComponentId"])
}

func TestRegistryInvalidRoot(t *testing.T) {
	info := exampleInfo()
	info.MSI.Registry = []nfpm.MSIRegistry{
		{Root: "HKXX", Key: `Software\TestCo`, Name: "x", Value: "y"},
	}
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "root")
}

// upgradeRows returns the Upgrade rows split into the remove-older row and the
// detect-newer (downgrade block) row, either of which may be nil.
func upgradeRows(t *testing.T, db gomsi.Database) (remove, detectNewer gomsi.Row) {
	t.Helper()
	for _, r := range tableRows(t, db, "Upgrade") {
		require.Equal(t, property(t, db, "UpgradeCode"), r["UpgradeCode"])
		if r["Attributes"].(int32)&upgradeOnlyDetect != 0 {
			detectNewer = r
		} else {
			remove = r
		}
	}
	return remove, detectNewer
}

// TestMajorUpgradeDefault proves a default configuration replaces older
// installs and blocks downgrades: the Upgrade table finds related products,
// RemoveExistingProducts removes them, and a newer install is refused.
func TestMajorUpgradeDefault(t *testing.T) {
	db := buildDB(t, exampleInfo())

	remove, detectNewer := upgradeRows(t, db)
	require.NotNil(t, remove, "older versions must be detected for removal")
	require.NotNil(t, detectNewer, "newer versions must be detected to block downgrades")
	require.Equal(t, "1.2.3", remove["VersionMax"])
	require.NotZero(t, remove["Attributes"].(int32)&upgradeMigrateFeatures)
	require.Zero(t, remove["Attributes"].(int32)&upgradeVersionMaxInclusive, "the same version is not an upgrade by default")
	require.Equal(t, "1.2.3", detectNewer["VersionMin"])

	seq := tableRows(t, db, "InstallExecuteSequence")
	rep := rowWhere(t, seq, "Action", "RemoveExistingProducts")
	files := rowWhere(t, seq, "Action", "InstallFiles")
	require.Less(t, rep["Sequence"].(int16), files["Sequence"].(int16),
		"the old product is removed before the new files land")

	conditions := tableRows(t, db, "LaunchCondition")
	require.Len(t, conditions, 1)
	require.Contains(t, conditions[0]["Condition"], detectNewer["ActionProperty"])
	require.Contains(t, conditions[0]["Description"], "newer version")
}

func TestMajorUpgradeOptions(t *testing.T) {
	t.Run("downgrade message", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.Upgrade.DowngradeErrorMessage = "A newer version of TestApp is already installed."
		db := buildDB(t, info)
		conditions := tableRows(t, db, "LaunchCondition")
		require.Len(t, conditions, 1)
		require.Equal(t, "A newer version of TestApp is already installed.", conditions[0]["Description"])
	})

	t.Run("allow downgrades", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.Upgrade.AllowDowngrades = true
		db := buildDB(t, info)
		remove, detectNewer := upgradeRows(t, db)
		require.NotNil(t, remove)
		require.Nil(t, detectNewer, "no downgrade detection")
		require.False(t, hasTable(db, "LaunchCondition"))
	})

	t.Run("allow same version", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.Upgrade.AllowSameVersion = true
		db := buildDB(t, info)
		remove, _ := upgradeRows(t, db)
		require.NotZero(t, remove["Attributes"].(int32)&upgradeVersionMaxInclusive,
			"an equal version must be in the removed range")
	})
}

// TestLicenseContent proves a shared contents entry of type license is both
// installed as a file and used as the install-UI license text.
func TestLicenseContent(t *testing.T) {
	dir := t.TempDir()
	license := filepath.Join(dir, "LICENSE.txt")
	require.NoError(t, os.WriteFile(license, []byte("Test license text"), 0o600))

	info := exampleInfo()
	info.MSI.MinimalUI = true
	info.Contents = append(info.Contents, &files.Content{
		Source:      license,
		Destination: "/Program Files/TestApp/LICENSE.txt",
		Type:        files.TypeRPMLicense,
	})
	db := buildDB(t, info)

	componentFor(t, db, "LICENSE.txt")
	var shown bool
	for _, c := range tableRows(t, db, "Control") {
		if text, _ := c["Text"].(string); strings.Contains(text, "Test license text") {
			shown = true
		}
	}
	require.True(t, shown, "license text must be shown by the install UI")
}

func TestMinimalUIOff(t *testing.T) {
	db := buildDB(t, exampleInfo())
	require.False(t, hasTable(db, "Dialog"), "no UI unless asked for")
}

func TestMissingLicenseFile(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "/does/not/exist/LICENSE.txt",
		Destination: "/Program Files/TestApp/LICENSE.txt",
		Type:        files.TypeRPMLicense,
	})
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
}

func TestSetPackagerDefaults(t *testing.T) {
	info := &nfpm.Info{
		Name: "MyApp",
		Overridables: nfpm.Overridables{
			MSI: nfpm.MSI{
				Manufacturer: "Co",
				Services:     []nfpm.MSIService{{Name: "S", Executable: "x"}},
				Shortcuts:    []nfpm.MSIShortcut{{Name: "S", Target: "x"}},
			},
		},
	}
	msi.Default.SetPackagerDefaults(info)

	require.Equal(t, "MyApp", info.MSI.ProductName)
	require.Equal(t, "Co", info.MSI.Manufacturer)
	require.Equal(t, "MyApp", info.MSI.InstallDir)
	require.False(t, info.MSI.PerUser)
	require.Equal(t, "demand", info.MSI.Services[0].StartType)
	require.Equal(t, "ProgramMenuFolder", info.MSI.Shortcuts[0].Directory)
}

// TestRootFieldProperties proves shared root metadata lands in the standard
// ARP property rows.
func TestRootFieldProperties(t *testing.T) {
	db := buildDB(t, exampleInfo())
	require.Equal(t, "Test application", property(t, db, "ARPCOMMENTS"))
	require.Equal(t, "https://example.com", property(t, db, "ARPURLINFOABOUT"))
}

func TestRootFieldPropertiesUserOverride(t *testing.T) {
	info := exampleInfo()
	info.MSI.Properties = map[string]string{"ARPCOMMENTS": "custom comment", "MYPROP": "value"}
	db := buildDB(t, info)
	require.Equal(t, "custom comment", property(t, db, "ARPCOMMENTS"),
		"root description must not override the user-provided property")
	require.Equal(t, "value", property(t, db, "MYPROP"))
}

// TestReservedProperties proves the properties nfpm derives from its own
// fields cannot be set through msi.properties (they would duplicate the
// Property primary key).
func TestReservedProperties(t *testing.T) {
	for key, field := range map[string]string{
		"ProductName":    "msi.product_name",
		"ProductVersion": "msi.version",
		"Manufacturer":   "msi.manufacturer",
		"ProductCode":    "msi.product_code",
		"UpgradeCode":    "msi.upgrade_code",
		"ALLUSERS":       "msi.per_user",
	} {
		t.Run(key, func(t *testing.T) {
			info := exampleInfo()
			info.MSI.Properties = map[string]string{key: "x"}
			err := msi.Default.Package(info, io.Discard)
			require.Error(t, err)
			require.Contains(t, err.Error(), `msi.properties["`+key+`"]`)
			require.Contains(t, err.Error(), field)
		})
	}

	t.Run("ProductLanguage", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.Properties = map[string]string{"ProductLanguage": "1031"}
		err := msi.Default.Package(info, io.Discard)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot be overridden")
	})

	t.Run("empty value", func(t *testing.T) {
		info := exampleInfo()
		info.MSI.Properties = map[string]string{"MYPROP": ""}
		err := msi.Default.Package(info, io.Discard)
		require.Error(t, err)
		require.Contains(t, err.Error(), `msi.properties["MYPROP"] must not be empty`)
	})
}

// TestDestinations covers the Windows destination edge cases: a bare drive is
// not a file, two spellings of one path must not both be packaged, and a
// drive-rooted directory destination is fine.
func TestDestinations(t *testing.T) {
	for _, dst := range []string{"C:", "/C:"} {
		t.Run("bare drive "+dst, func(t *testing.T) {
			info := exampleInfo()
			info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: dst}}
			err := msi.Default.Package(info, io.Discard)
			require.Error(t, err)
			require.Contains(t, err.Error(), "is empty once the drive letter is removed")
		})
	}

	t.Run("colliding spellings", func(t *testing.T) {
		info := exampleInfo()
		info.Contents = append(info.Contents, &files.Content{
			Source:      "../testdata/whatever.conf",
			Destination: "C:/Program Files/TestApp/app.exe",
		})
		err := msi.Default.Package(info, io.Discard)
		require.Error(t, err)
		require.Contains(t, err.Error(), "resolve to the same Windows path")
	})

	t.Run("drive root directory", func(t *testing.T) {
		info := exampleInfo()
		info.Contents = []*files.Content{{Source: "../testdata/whatever.conf", Destination: "C:/"}}
		db := buildDB(t, info)
		comp := componentFor(t, db, "whatever.conf")
		require.Equal(t, "INSTALLFOLDER", comp["Directory_"])
	})

	t.Run("drive letter is ignored", func(t *testing.T) {
		info := exampleInfo()
		info.Contents = []*files.Content{{Source: "../testdata/fake", Destination: `C:\Program Files\TestApp\app.exe`}}
		db := buildDB(t, info)
		comp := componentFor(t, db, "app.exe")
		dir := rowWhere(t, tableRows(t, db, "Directory"), "Directory", comp["Directory_"])
		require.Equal(t, "ProgramFiles64Folder", dir["Directory_Parent"])
	})
}

// customAction returns the CustomAction row and its InstallExecuteSequence
// row for the given action.
func customAction(t *testing.T, db gomsi.Database, action string) (gomsi.Row, gomsi.Row) {
	t.Helper()
	ca := rowWhere(t, tableRows(t, db, "CustomAction"), "Action", action)
	seq := rowWhere(t, tableRows(t, db, "InstallExecuteSequence"), "Action", action)
	return ca, seq
}

// TestScriptsCustomActions proves each maintainer script becomes a deferred,
// elevated custom action scheduled around file installation or removal with
// the conditions that give it deb/rpm hook semantics.
func TestScriptsCustomActions(t *testing.T) {
	dir := t.TempDir()
	ps1 := filepath.Join(dir, "hook.ps1")
	bat := filepath.Join(dir, "hook.bat")
	require.NoError(t, os.WriteFile(ps1, []byte("Write-Output 'hello'\n"), 0o600))
	require.NoError(t, os.WriteFile(bat, []byte("@echo off\r\necho hello\r\n"), 0o600))

	info := exampleInfo()
	info.Scripts = nfpm.Scripts{
		PreInstall:  ps1,
		PostInstall: bat,
		PreRemove:   ps1,
		PostRemove:  ps1,
	}
	db := buildDB(t, info)
	seq := tableRows(t, db, "InstallExecuteSequence")
	installFiles := rowWhere(t, seq, "Action", "InstallFiles")["Sequence"].(int16)
	removeFiles := rowWhere(t, seq, "Action", "RemoveFiles")["Sequence"].(int16)

	for _, tt := range []struct {
		action    string
		before    int16
		after     int16
		condition string
	}{
		{"NfpmPreInstall", installFiles, 0, "NOT Installed"},
		{"NfpmPostInstall", 0, installFiles, "NOT Installed"},
		{"NfpmPreRemove", removeFiles, 0, `REMOVE="ALL" AND NOT UPGRADINGPRODUCTCODE`},
		{"NfpmPostRemove", 0, removeFiles, `REMOVE="ALL" AND NOT UPGRADINGPRODUCTCODE`},
	} {
		t.Run(tt.action, func(t *testing.T) {
			ca, s := customAction(t, db, tt.action)
			typ := ca["Type"].(int16)
			require.Equal(t, caTypeEXEInDir, typ&caTypeMask, "an executable run from a directory")
			require.NotZero(t, typ&caDeferred, "deferred")
			require.NotZero(t, typ&caNoImpersonate, "runs as SYSTEM")
			require.NotZero(t, typ&caHideTarget, "the embedded script stays out of the log")
			require.Equal(t, "TARGETDIR", ca["Source"])
			require.Contains(t, ca["Target"], "powershell.exe")
			require.Contains(t, ca["Target"], "-EncodedCommand")

			require.Equal(t, tt.condition, s["Condition"])
			sequence := s["Sequence"].(int16)
			if tt.before != 0 {
				require.Less(t, sequence, tt.before)
			}
			if tt.after != 0 {
				require.Greater(t, sequence, tt.after)
			}
		})
	}
}

func TestScriptUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(sh, []byte("#!/bin/sh\n"), 0o600))

	info := exampleInfo()
	info.Scripts.PreInstall = sh
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), ".ps1, .bat, or .cmd")
}

func TestScriptMissingFile(t *testing.T) {
	info := exampleInfo()
	info.Scripts.PostInstall = "/does/not/exist.ps1"
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scripts.postinstall")
}

// TestScriptUnreadable covers the read failure in addScripts. A directory named
// hook.ps1 clears validateScripts — it stats fine, is under the size limit and
// has a supported extension — then fails when the body is read.
func TestScriptUnreadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hook.ps1")
	require.NoError(t, os.Mkdir(dir, 0o700))

	info := exampleInfo()
	info.Scripts.PreInstall = dir
	err := msi.Default.Package(info, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "scripts.preinstall")
}

// TestScriptSizeLimit pins both sides of the size limit: the largest accepted
// script builds, one byte more is rejected.
func TestScriptSizeLimit(t *testing.T) {
	dir := t.TempDir()
	const limit = 8 * 1024

	largest := filepath.Join(dir, "largest.ps1")
	require.NoError(t, os.WriteFile(largest, bytes.Repeat([]byte("#"), limit), 0o600))
	info := exampleInfo()
	info.Scripts.PreRemove = largest
	buildDB(t, info)

	big := filepath.Join(dir, "big.ps1")
	require.NoError(t, os.WriteFile(big, bytes.Repeat([]byte("#"), limit+1), 0o600))
	info = exampleInfo()
	info.Scripts.PreRemove = big
	err := msi.Default.Package(info, io.Discard)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most")
}

func TestSigning(t *testing.T) {
	pfxPath, passphrase := makeTestPFX(t)

	info := exampleInfo()
	info.MSI.Signature = nfpm.MSISignature{
		PFXFile:       pfxPath,
		KeyPassphrase: passphrase,
	}

	var buf bytes.Buffer
	require.NoError(t, msi.Default.Package(info, &buf))
	require.True(t, bytes.HasPrefix(buf.Bytes(), cfbMagic))

	_, err := gomsi.Verify(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err, "signed MSI should verify")
}

func TestSigningMissingPFX(t *testing.T) {
	info := exampleInfo()
	info.MSI.Signature.PFXFile = "/does/not/exist.pfx"
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PFX file not found")
}

// TestSigningWrongPassphrase proves a PFX that exists but cannot be opened
// surfaces as a signing failure rather than a generic error.
func TestSigningWrongPassphrase(t *testing.T) {
	pfxPath, _ := makeTestPFX(t)

	info := exampleInfo()
	info.MSI.Signature = nfpm.MSISignature{
		PFXFile:       pfxPath,
		KeyPassphrase: "wrong-passphrase",
	}

	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)

	var expectedError *nfpm.ErrSigningFailure
	require.ErrorAs(t, err, &expectedError)
}

// TestSigningPFXNotAccessible covers the stat failure that is not ErrNotExist:
// a path whose parent is a regular file yields ENOTDIR on unix. Windows reports
// that as ERROR_PATH_NOT_FOUND, which maps to fs.ErrNotExist and takes the
// not-found branch instead, so the distinction only exists off Windows.
func TestSigningPFXNotAccessible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows maps a non-directory path component to ErrNotExist")
	}

	info := exampleInfo()
	info.MSI.Signature.PFXFile = filepath.Join("../testdata/whatever.conf", "nope.pfx")

	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unable to access PFX file")
}

// TestSigningWithTimestampURL exercises the timestamp branch of the signer
// build. Building the signer does not contact the network; the URL is only
// fetched later, while writing the package, so a stub server that rejects the
// request proves the URL was plumbed through and reached.
func TestSigningWithTimestampURL(t *testing.T) {
	pfxPath, passphrase := makeTestPFX(t)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	info := exampleInfo()
	info.MSI.Signature = nfpm.MSISignature{
		PFXFile:       pfxPath,
		KeyPassphrase: passphrase,
		TimestampURL:  srv.URL,
	}

	err := msi.Default.Package(info, io.Discard)
	require.Error(t, err, "a failing timestamp authority must fail the package")
	require.Positive(t, hits.Load(), "the configured timestamp URL must actually be requested")
}

// errWriter fails every write, to exercise the package-writing error path.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteMSIError(t *testing.T) {
	err := msi.Default.Package(exampleInfo(), errWriter{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "write failed")
}

func TestValidateErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*nfpm.Info)
		expect string
	}{
		{
			name: "no manufacturer, vendor or maintainer",
			mutate: func(info *nfpm.Info) {
				info.MSI.Manufacturer = ""
				info.Vendor = ""
				info.Maintainer = ""
			},
			expect: "must be provided",
		},
		{
			name: "shortcut without a name",
			mutate: func(info *nfpm.Info) {
				info.MSI.Shortcuts = []nfpm.MSIShortcut{
					{Target: "/Program Files/TestApp/app.exe"},
				}
			},
			expect: "msi.shortcuts[0].name",
		},
		{
			name: "shortcut without a target",
			mutate: func(info *nfpm.Info) {
				info.MSI.Shortcuts = []nfpm.MSIShortcut{{Name: "Test App"}}
			},
			expect: "msi.shortcuts[0].target",
		},
		{
			name: "service without a name",
			mutate: func(info *nfpm.Info) {
				info.MSI.Services = []nfpm.MSIService{
					{Executable: "/Program Files/TestApp/app.exe", StartType: "demand"},
				}
			},
			expect: "msi.services[0].name",
		},
		{
			name: "service without an executable",
			mutate: func(info *nfpm.Info) {
				info.MSI.Services = []nfpm.MSIService{{Name: "TestSvc", StartType: "demand"}}
			},
			expect: "msi.services[0].executable",
		},
		{
			name: "registry entry without a key",
			mutate: func(info *nfpm.Info) {
				info.MSI.Registry = []nfpm.MSIRegistry{{Root: "HKLM", Name: "x", Value: "y"}}
			},
			expect: "msi.registry[0].key",
		},
		{
			name: "invalid upgrade code",
			mutate: func(info *nfpm.Info) {
				info.MSI.UpgradeCode = "not-a-guid"
			},
			expect: "msi.upgrade_code",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := exampleInfo()
			tt.mutate(info)
			err := msi.Default.Package(info, io.Discard)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expect)
		})
	}
}

// TestPackage386 builds a 32-bit package, covering the 32-bit arms of the
// architecture-dependent helpers in one pass: the ProgramFilesFolder mapping,
// the component attributes, and the [SystemFolder] script interpreter path.
func TestPackage386(t *testing.T) {
	dir := t.TempDir()
	ps1 := filepath.Join(dir, "hook.ps1")
	require.NoError(t, os.WriteFile(ps1, []byte("Write-Output 'hi'\n"), 0o600))

	info := exampleInfo()
	info.Arch = "386"
	info.Scripts.PreInstall = ps1

	db := buildDB(t, info)
	for _, c := range tableRows(t, db, "Component") {
		require.Zero(t, c["Attributes"].(int16)&attr64bit, "no 64-bit components in a 32-bit package")
	}
	ca, _ := customAction(t, db, "NfpmPreInstall")
	require.Contains(t, ca["Target"], "[SystemFolder]")
}

// TestComponentBitness proves the 64-bit component attribute follows the
// folder, not the package: a 64-bit package still installs 32-bit content
// into Program Files (x86) and SysWOW64 (ICE80).
func TestComponentBitness(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents,
		&files.Content{Source: "../testdata/fake", Destination: "/Program Files (x86)/TestApp/legacy.exe"},
		&files.Content{Source: "../testdata/fake", Destination: "/Windows/SysWOW64/legacy.dll"},
	)
	db := buildDB(t, info)

	require.NotZero(t, componentFor(t, db, "app.exe")["Attributes"].(int16)&attr64bit)
	require.NotZero(t, componentFor(t, db, "config.conf")["Attributes"].(int16)&attr64bit)
	legacy := componentFor(t, db, "legacy.exe")
	require.Zero(t, legacy["Attributes"].(int16)&attr64bit)
	require.Equal(t, "ProgramFilesFolder",
		rowWhere(t, tableRows(t, db, "Directory"), "Directory", legacy["Directory_"])["Directory_Parent"])
	wow := componentFor(t, db, "legacy.dll")
	require.Zero(t, wow["Attributes"].(int16)&attr64bit)
	require.Equal(t, "SystemFolder", wow["Directory_"])
}

// TestPackageSystemDir proves files installed into a Windows system directory
// are marked permanent, which ICE09 requires, and that the user is told those
// files will outlive the product.
func TestPackageSystemDir(t *testing.T) {
	logs := captureLogs(t)
	info := exampleInfo()
	info.Contents = append(info.Contents,
		&files.Content{Source: "../testdata/fake", Destination: "/Windows/System32/testapp.dll"},
		&files.Content{Source: "../testdata/fake", Destination: "/Windows/System32/other.dll"},
	)
	db := buildDB(t, info)

	comp := componentFor(t, db, "testapp.dll")
	require.Equal(t, "System64Folder", comp["Directory_"])
	require.NotZero(t, comp["Attributes"].(int16)&attrPermanent)
	require.Zero(t, componentFor(t, db, "app.exe")["Attributes"].(int16)&attrPermanent)

	require.Contains(t, logs.String(), "never removes such files")
	require.Equal(t, 1, strings.Count(logs.String(), "never removes such files"), "warned once per system folder")
}

func TestSymlinkSkipped(t *testing.T) {
	logs := captureLogs(t)

	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "/Program Files/TestApp/app.exe",
		Destination: "/Program Files/TestApp/link.exe",
		Type:        files.TypeSymlink,
	})
	db := buildDB(t, info)
	require.Len(t, tableRows(t, db, "File"), 2)
	require.Contains(t, logs.String(), "msi does not support symlinks")
}

func TestConfigContentWarns(t *testing.T) {
	logs := captureLogs(t)

	info := exampleInfo()
	info.Contents[1].Type = files.TypeConfigNoReplace
	db := buildDB(t, info)
	componentFor(t, db, "config.conf")
	require.Contains(t, logs.String(), "installed as a regular file")
}

func TestDirectoryContentSkipped(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Destination: "/Program Files/TestApp/data",
		Type:        files.TypeDir,
	})
	db := buildDB(t, info)
	require.Len(t, tableRows(t, db, "File"), 2)
}

// TestMultipleLicenses proves the first license wins for the install UI and the
// rest are still installed as ordinary files.
func TestMultipleLicenses(t *testing.T) {
	logs := captureLogs(t)

	dir := t.TempDir()
	first := filepath.Join(dir, "LICENSE.txt")
	second := filepath.Join(dir, "LICENSE2.txt")
	require.NoError(t, os.WriteFile(first, []byte("First license text"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("Second license text"), 0o600))

	info := exampleInfo()
	info.MSI.MinimalUI = true
	info.Contents = append(info.Contents,
		&files.Content{
			Source:      first,
			Destination: "/Program Files/TestApp/LICENSE.txt",
			Type:        files.TypeRPMLicense,
		},
		&files.Content{
			Source:      second,
			Destination: "/Program Files/TestApp/LICENSE2.txt",
			Type:        files.TypeRPMLicense,
		},
	)

	db := buildDB(t, info)
	componentFor(t, db, "LICENSE.txt")
	componentFor(t, db, "LICENSE2.txt")
	var firstShown, secondShown bool
	for _, c := range tableRows(t, db, "Control") {
		text, _ := c["Text"].(string)
		firstShown = firstShown || strings.Contains(text, "First license text")
		secondShown = secondShown || strings.Contains(text, "Second license text")
	}
	require.True(t, firstShown, "the first license feeds the UI")
	require.False(t, secondShown)
	require.Contains(t, logs.String(), "multiple license contents")
}

func TestMissingContentSource(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "/does/not/exist/app.exe",
		Destination: "/Program Files/TestApp/missing.exe",
	})
	err := msi.Default.Package(info, io.Discard)
	require.Error(t, err)
}

// TestNoRootMetadata covers the arms where the ARP properties are skipped
// because the corresponding root fields are empty.
func TestNoRootMetadata(t *testing.T) {
	info := exampleInfo()
	info.Description = ""
	info.Homepage = ""
	db := buildDB(t, info)
	require.Empty(t, property(t, db, "ARPCOMMENTS"))
	require.Empty(t, property(t, db, "ARPURLINFOABOUT"))
}

// makeTestPFX generates a self-signed code-signing cert and writes it as a
// password-protected PKCS#12 file, returning its path and passphrase.
func makeTestPFX(t *testing.T) (string, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "nfpm-msi-test"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(0, 0).AddDate(20, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	const passphrase = "test123"
	pfx, err := pkcs12.Modern.Encode(key, cert, nil, passphrase)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "test.pfx")
	require.NoError(t, os.WriteFile(path, pfx, 0o600))
	return path, passphrase
}
