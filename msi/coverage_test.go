package msi

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
	gomsi "go.digitalxero.dev/go-msi"
)

func TestIs64bit(t *testing.T) {
	for _, tt := range []struct {
		arch string
		want bool
	}{
		{"x64", true},
		{"arm64", true},
		{"x86", false},
		{"arm", false},
		{"neutral", false},
		{"", false},
	} {
		t.Run(tt.arch, func(t *testing.T) {
			require.Equal(t, tt.want, is64bit(tt.arch))
		})
	}
}

func TestEnsureValidArchUnknown(t *testing.T) {
	info := &nfpm.Info{Arch: "riscv64"}
	require.Equal(t, "riscv64", ensureValidArch(info).Arch,
		"an arch absent from archToMSI must pass through untouched")
}

func TestVendorOrMaintainer(t *testing.T) {
	for _, tt := range []struct {
		name       string
		vendor     string
		maintainer string
		want       string
	}{
		{"vendor wins", "TestCo", "Test <test@example.com>", "TestCo"},
		{"maintainer with email", "", "Test <test@example.com>", "Test"},
		{"maintainer without email", "", "Jane Doe", "Jane Doe"},
		{"neither", "", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, vendorOrMaintainer(&nfpm.Info{
				Vendor:     tt.vendor,
				Maintainer: tt.maintainer,
			}))
		})
	}
}

func TestNormalizeDest(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"/app/config.conf", "app/config.conf"},
		{`\app\config.conf`, "app/config.conf"},
		{"C:/Program Files/app.exe", "Program Files/app.exe"},
		{"/C:/Program Files/app.exe", "Program Files/app.exe"},
		{`C:\Program Files\app.exe`, "Program Files/app.exe"},
		{"//app//nested//config.conf", "app/nested/config.conf"},
		{"app/config.conf", "app/config.conf"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeDest(tt.in))
		})
	}
}

func TestMapDestination(t *testing.T) {
	for _, tt := range []struct {
		name        string
		dest        string
		is64        bool
		wantRoot    string
		wantDefault string
		wantRel     string
	}{
		{
			name: "program files 64bit", dest: "Program Files/TestApp/app.exe", is64: true,
			wantRoot: "ProgramFiles64Folder", wantDefault: ".", wantRel: "TestApp/app.exe",
		},
		{
			name: "program files 32bit", dest: "Program Files/TestApp/app.exe", is64: false,
			wantRoot: "ProgramFilesFolder", wantDefault: ".", wantRel: "TestApp/app.exe",
		},
		{
			name: "program files x86 stays 32bit", dest: "Program Files (x86)/TestApp/app.exe", is64: true,
			wantRoot: "ProgramFilesFolder", wantDefault: ".", wantRel: "TestApp/app.exe",
		},
		{
			name: "system32 64bit", dest: "Windows/System32/driver.dll", is64: true,
			wantRoot: "System64Folder", wantDefault: ".", wantRel: "driver.dll",
		},
		{
			name: "system32 32bit", dest: "Windows/System32/driver.dll", is64: false,
			wantRoot: "SystemFolder", wantDefault: ".", wantRel: "driver.dll",
		},
		{
			// An exact prefix match has no remainder, so the base name is used.
			name: "exact prefix match falls back to base", dest: "Program Files", is64: true,
			wantRoot: "ProgramFiles64Folder", wantDefault: ".", wantRel: "Program Files",
		},
		{
			name: "unknown prefix falls back to INSTALLFOLDER", dest: "app/config.conf", is64: true,
			wantRoot: "INSTALLFOLDER", wantDefault: "", wantRel: "app/config.conf",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, def, rel := mapDestination(tt.dest, tt.is64)
			require.Equal(t, tt.wantRoot, root)
			require.Equal(t, tt.wantDefault, def)
			require.Equal(t, tt.wantRel, rel)
		})
	}
}

func TestPlatformFor(t *testing.T) {
	for _, tt := range []struct {
		arch string
		want gomsi.Platform
		ok   bool
		is64 bool
	}{
		{"x64", gomsi.Platform_x64, true, true},
		{"x86", gomsi.Platform_Intel, true, false},
		{"intel", gomsi.Platform_Intel, true, false},
		{"arm", gomsi.Platform_Arm, true, false},
		{"arm64", gomsi.Platform_Arm64, true, true},
		// Intel64 is Itanium, not x86-64, but it is still a 64-bit target.
		{"intel64", gomsi.Platform_Intel64, true, true},
		// Windows Installer accepts variant casing.
		{"X64", gomsi.Platform_x64, true, true},
		{"ARM64", gomsi.Platform_Arm64, true, true},
		// neutral is an MSIX architecture with no MSI equivalent.
		{"neutral", 0, false, false},
		{"amd64", 0, false, false},
		{"ppc64le", 0, false, false},
		{"", 0, false, false},
	} {
		t.Run(tt.arch, func(t *testing.T) {
			got, ok := platformFor(tt.arch)
			require.Equal(t, tt.ok, ok)
			if tt.ok {
				require.Equal(t, tt.want, got)
			}
			require.Equal(t, tt.is64, is64bit(tt.arch))
		})
	}
}

func TestComponentAttributes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		rootID string
		is64   bool
		want   int16
	}{
		{"32bit installfolder has no attributes", "INSTALLFOLDER", false, 0},
		{"64bit installfolder", "INSTALLFOLDER", true, msidbComponentAttributes64bit},
		{"32bit system dir is permanent", "SystemFolder", false, msidbComponentAttributesPermanent},
		{
			"64bit system dir is permanent and 64bit", "System64Folder", true,
			msidbComponentAttributes64bit | msidbComponentAttributesPermanent,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, componentAttributes(tt.rootID, tt.is64))
		})
	}
}

func TestMakeID(t *testing.T) {
	t.Run("truncates long readable segments", func(t *testing.T) {
		long := strings.Repeat("a", 60) + ".exe"
		id := makeID("c", "/app/"+long)
		require.LessOrEqual(t, len(id), 40+len("c_")+9,
			"the readable portion must be truncated to 40 chars")
		require.Contains(t, id, strings.Repeat("a", 40))
	})

	t.Run("is stable for the same seed", func(t *testing.T) {
		first := makeID("c", "/app/x.exe")
		second := makeID("c", "/app/x.exe")
		require.Equal(t, first, second)
	})

	t.Run("differs for different seeds", func(t *testing.T) {
		require.NotEqual(t, makeID("c", "/app/x.exe"), makeID("c", "/app/y.exe"))
	})
}

func TestSanitizeID(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"app.exe", "app.exe"},
		{"my file.exe", "my_file.exe"},
		{"my-file-v2.exe", "my_file_v2.exe"},
		{"under_score", "under_score"},
		{"Mixed123", "Mixed123"},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, sanitizeID(tt.in))
		})
	}
}

// Windows Installer bounds ProductVersion per field: major and minor at most
// 255, build at most 65535.
func TestConvertToMSIVersionClamp(t *testing.T) {
	for _, tt := range []struct {
		in      string
		want    string
		clamped bool
	}{
		{"99999.1.2", "255.1.2", true},
		{"1.99999.2", "1.255.2", true},
		{"1.2.99999", "1.2.65535", true},
		{"256.256.65536", "255.255.65535", true},
		{"2024.1.0", "255.1.0", true},
		{"255.255.65535", "255.255.65535", false},
		{"1.2.3", "1.2.3", false},
		{"1", "1.0.0", false},
		{"notanumber", "0.0.0", false},
		{"1.x.3", "1.0.3", false},
		{"v1.2.3-rc1+meta", "1.2.3", false},
		{"-1.2.3", "0.0.0", false},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, convertToMSIVersion(tt.in))

			converted, clamped := convertToMSIVersionClamped(tt.in)
			require.Equal(t, tt.want, converted)
			require.Equal(t, tt.clamped, clamped)
		})
	}
}

// No field can go negative: the sign is stripped along with the pre-release
// suffix before any field is parsed.
func TestConvertToMSIVersionNeverNegative(t *testing.T) {
	for _, in := range []string{"-1.0.0", "1.-5.0", "1.2.-3", "-1.-2.-3"} {
		t.Run(in, func(t *testing.T) {
			require.NotContains(t, convertToMSIVersion(in), "-")
		})
	}
}

func TestMSIVersionOverride(t *testing.T) {
	t.Run("overrides the shared version", func(t *testing.T) {
		info := derivationInfo("TestCo", "TestApp", "x64", "2024.1.0")
		info.MSI.Version = "24.1.0"

		converted, clamped := msiVersionClamped(info)
		require.Equal(t, "24.1.0", converted)
		require.False(t, clamped, "an in-range override must not report clamping")
	})

	t.Run("falls back to the shared version", func(t *testing.T) {
		info := derivationInfo("TestCo", "TestApp", "x64", "1.2.3")

		require.Equal(t, "1.2.3", msiVersion(info))
	})

	t.Run("is itself bounded", func(t *testing.T) {
		info := derivationInfo("TestCo", "TestApp", "x64", "1.2.3")
		info.MSI.Version = "2024.1.0"

		converted, clamped := msiVersionClamped(info)
		require.Equal(t, "255.1.0", converted)
		require.True(t, clamped)
	})

	t.Run("participates in the derived ProductCode", func(t *testing.T) {
		base := derivationInfo("TestCo", "TestApp", "x64", "1.2.3")

		overridden := derivationInfo("TestCo", "TestApp", "x64", "1.2.3")
		overridden.MSI.Version = "2.0.0"

		require.NotEqual(t, deriveProductCode(base), deriveProductCode(overridden),
			"the ProductCode must follow the version the package actually declares")
		require.Equal(t, deriveUpgradeCode(base), deriveUpgradeCode(overridden),
			"the UpgradeCode stays version independent")
	})
}

func TestLooksLikeGUID(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want bool
	}{
		{"{12345678-1234-1234-1234-123456789ABC}", true},
		{"{12345678-1234-1234-1234-123456789abc}", true},
		{"12345678-1234-1234-1234-123456789ABC", false},
		{"{not-a-guid}", false},
		{"", false},
	} {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, looksLikeGUID(tt.in))
		})
	}
}

func TestDeriveGUID(t *testing.T) {
	got := deriveGUID("product|TestApp")
	require.True(t, looksLikeGUID(got), "derived GUID %q must be a braced GUID", got)
	require.Equal(t, got, deriveGUID("product|TestApp"), "derivation must be stable")
	require.NotEqual(t, got, deriveGUID("upgrade|TestApp"))
}

func derivationInfo(manufacturer, name, arch, version string) *nfpm.Info {
	return &nfpm.Info{
		Name:    name,
		Arch:    arch,
		Version: version,
		Overridables: nfpm.Overridables{
			MSI: nfpm.MSI{
				ProductName:  name,
				Manufacturer: manufacturer,
			},
		},
	}
}

// TestDerivedCodeNamespaces pins the properties Windows Installer relies on:
// the ProductCode must change on every release and differ per manufacturer,
// product, and arch, while the UpgradeCode must stay stable across releases
// but still differ per manufacturer, product, and arch.
func TestDerivedCodeNamespaces(t *testing.T) {
	base := derivationInfo("TestCo", "TestApp", "x64", "1.2.3")

	for _, code := range []string{deriveProductCode(base), deriveUpgradeCode(base)} {
		require.True(t, looksLikeGUID(code), "derived code %q must be a braced GUID", code)
	}
	require.NotEqual(t, deriveProductCode(base), deriveUpgradeCode(base))

	t.Run("new version", func(t *testing.T) {
		next := derivationInfo("TestCo", "TestApp", "x64", "2.0.0")
		require.NotEqual(t, deriveProductCode(base), deriveProductCode(next),
			"ProductCode must change on every release for major upgrades to work")
		require.Equal(t, deriveUpgradeCode(base), deriveUpgradeCode(next),
			"UpgradeCode must stay stable across releases")
	})

	t.Run("same MSI version", func(t *testing.T) {
		// Pre-release/metadata are dropped by convertToMSIVersion, so versions
		// MSI cannot tell apart must share a ProductCode.
		same := derivationInfo("TestCo", "TestApp", "x64", "v1.2.3-rc1+meta")
		require.Equal(t, deriveProductCode(base), deriveProductCode(same))
	})

	t.Run("other arch", func(t *testing.T) {
		arm := derivationInfo("TestCo", "TestApp", "arm64", "1.2.3")
		require.NotEqual(t, deriveProductCode(base), deriveProductCode(arm))
		require.NotEqual(t, deriveUpgradeCode(base), deriveUpgradeCode(arm),
			"each arch is its own product line")
	})

	t.Run("other manufacturer", func(t *testing.T) {
		other := derivationInfo("OtherCo", "TestApp", "x64", "1.2.3")
		require.NotEqual(t, deriveProductCode(base), deriveProductCode(other),
			"unrelated vendors sharing a product name must not collide")
		require.NotEqual(t, deriveUpgradeCode(base), deriveUpgradeCode(other))
	})

	t.Run("other product", func(t *testing.T) {
		other := derivationInfo("TestCo", "OtherApp", "x64", "1.2.3")
		require.NotEqual(t, deriveProductCode(base), deriveProductCode(other))
		require.NotEqual(t, deriveUpgradeCode(base), deriveUpgradeCode(other))
	})
}

// TestAddContentsSkipsEmptySource covers the empty-source guard. Package cannot
// reach it: the only content type with no source is TypeRPMGhost, which the
// files package filters out for msi before addContents ever runs.
func TestAddContentsSkipsEmptySource(t *testing.T) {
	info := &nfpm.Info{
		Arch: "x64",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{Destination: "/Program Files/TestApp/app.exe"},
			},
		},
	}
	placed, err := addContents(gomsi.NewPackage(), info)
	require.NoError(t, err)
	require.Empty(t, placed, "a content with no source must not be placed")
}

// TestAddContentsUnreadableSource covers the FileSourceFromPath error. Package
// cannot reach it because PrepareForPackager rejects a missing source first.
func TestAddContentsUnreadableSource(t *testing.T) {
	info := &nfpm.Info{
		Arch: "x64",
		Overridables: nfpm.Overridables{
			Contents: []*files.Content{
				{Source: "/does/not/exist/app.exe", Destination: "/Program Files/TestApp/app.exe"},
			},
		},
	}
	_, err := addContents(gomsi.NewPackage(), info)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading file")
}

// TestAddShortcutsTargetNotPlaced covers the defensive guard in addShortcuts.
// Package cannot reach it because validate rejects an unknown shortcut target
// first, so the path is only reachable by calling addShortcuts directly.
func TestAddShortcutsTargetNotPlaced(t *testing.T) {
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			MSI: nfpm.MSI{
				Shortcuts: []nfpm.MSIShortcut{
					{Name: "Test App", Target: "/Program Files/TestApp/app.exe"},
				},
			},
		},
	}
	err := addShortcuts(nil, info, map[string]placement{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "was not installed")
}

// TestAddServicesExecutableNotPlaced covers the sibling defensive guard in
// addServices, likewise pre-empted by validate in the Package flow.
func TestAddServicesExecutableNotPlaced(t *testing.T) {
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			MSI: nfpm.MSI{
				Services: []nfpm.MSIService{
					{Name: "TestSvc", Executable: "/Program Files/TestApp/svc.exe"},
				},
			},
		},
	}
	err := addServices(nil, info, map[string]placement{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "was not installed")
}

func TestScriptCommandLineSystemFolder(t *testing.T) {
	dir := t.TempDir()
	ps1 := filepath.Join(dir, "hook.ps1")
	require.NoError(t, os.WriteFile(ps1, []byte("Write-Output 'hi'\n"), 0o600))

	t.Run("64bit uses System64Folder", func(t *testing.T) {
		cmd, err := scriptCommandLine("NfpmPreInstall", ps1, true)
		require.NoError(t, err)
		require.Contains(t, cmd, `[System64Folder]WindowsPowerShell\v1.0\powershell.exe`)
	})

	t.Run("32bit uses SystemFolder", func(t *testing.T) {
		cmd, err := scriptCommandLine("NfpmPreInstall", ps1, false)
		require.NoError(t, err)
		require.Contains(t, cmd, `[SystemFolder]WindowsPowerShell\v1.0\powershell.exe`)
		require.NotContains(t, cmd, "[System64Folder]")
	})
}

// TestScriptCommandLineEmbedsScript proves the script body travels base64 inside
// the UTF-16LE -EncodedCommand bootstrap, which is what lets it run at points
// where the payload is not on disk.
func TestScriptCommandLineEmbedsScript(t *testing.T) {
	dir := t.TempDir()
	ps1 := filepath.Join(dir, "hook.ps1")
	require.NoError(t, os.WriteFile(ps1, []byte("Write-Output 'marker'\n"), 0o600))

	cmd, err := scriptCommandLine("NfpmPreInstall", ps1, true)
	require.NoError(t, err)

	_, encoded, ok := strings.Cut(cmd, "-EncodedCommand ")
	require.True(t, ok, "command line must carry an -EncodedCommand payload")

	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)

	// Decode the UTF-16LE bootstrap back to a comparable string.
	var sb strings.Builder
	for i := 0; i+1 < len(raw); i += 2 {
		sb.WriteRune(rune(uint16(raw[i]) | uint16(raw[i+1])<<8))
	}
	bootstrap := sb.String()

	require.Contains(t, bootstrap, "nfpm_NfpmPreInstall.ps1")
	require.Contains(t, bootstrap, base64.StdEncoding.EncodeToString([]byte("Write-Output 'marker'\n")),
		"the script body must be embedded base64 in the bootstrap")
	require.Contains(t, bootstrap, "Remove-Item -Force", "the bootstrap must clean up after itself")
}

// TestScriptCommandLineUnsupportedExtension covers the defensive extension check
// in scriptCommandLine; validateScripts rejects the same condition earlier, so
// Package never reaches it.
func TestScriptCommandLineUnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(sh, []byte("#!/bin/sh\n"), 0o600))

	_, err := scriptCommandLine("NfpmPreInstall", sh, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), ".ps1, .bat, or .cmd")
}

// TestScriptCommandLineUnreadable covers the os.ReadFile error. A directory
// named hook.ps1 passes validateScripts (it stats fine, is under the size limit
// and has a supported extension) but fails to read.
func TestScriptCommandLineUnreadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hook.ps1")
	require.NoError(t, os.Mkdir(dir, 0o700))

	_, err := scriptCommandLine("NfpmPreInstall", dir, true)
	require.Error(t, err)
}

func TestScriptHooks(t *testing.T) {
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			Scripts: nfpm.Scripts{
				PreInstall:  "pre.ps1",
				PostInstall: "post.ps1",
				PreRemove:   "preremove.ps1",
				PostRemove:  "postremove.ps1",
			},
		},
	}
	hooks := scriptHooks(info)
	require.Len(t, hooks, 4)

	byField := map[string]string{}
	for _, h := range hooks {
		byField[h.field] = h.caID
	}
	require.Equal(t, map[string]string{
		"preinstall":  "NfpmPreInstall",
		"postinstall": "NfpmPostInstall",
		"preremove":   "NfpmPreRemove",
		"postremove":  "NfpmPostRemove",
	}, byField)
}

func TestUTF16LE(t *testing.T) {
	require.Equal(t, []byte{'h', 0, 'i', 0}, utf16le("hi"))
	require.Empty(t, utf16le(""))
}
