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
	"regexp"
	"runtime"
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
		"all":     "neutral",
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			info := exampleInfo()
			info.Arch = in
			name := msi.Default.ConventionalFileName(info)
			require.Contains(t, name, "_"+want+".msi")
		})
	}
}

func TestArchOverride(t *testing.T) {
	info := exampleInfo()
	info.Arch = "amd64"
	info.MSI.Arch = "x86"
	require.Contains(t, msi.Default.ConventionalFileName(info), "_x86.msi")
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

func TestPackageMinimal(t *testing.T) {
	packageAndValidate(t, exampleInfo())
}

func TestPackageWithContents(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents,
		&files.Content{Source: "../testdata/whatever.conf", Destination: "/Program Files/TestApp/sub/extra.txt"},
		&files.Content{Source: "../testdata/whatever.conf", Destination: "relative/path/file.txt"},
	)
	packageAndValidate(t, info)
}

// TestManufacturerFallback proves an MSI builds without any msi-specific
// manufacturer: the root vendor is embedded instead.
func TestManufacturerFallback(t *testing.T) {
	info := exampleInfo()
	info.MSI.Manufacturer = ""
	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("TestCo")), "vendor must be embedded as the manufacturer")
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
	packageAndValidate(t, info)
}

// TestLowercaseGUIDs proves that a canonical but lowercase GUID is accepted:
// validation is case-insensitive and the codes are normalized to uppercase
// before go-msi (which requires uppercase) sees them.
func TestLowercaseGUIDs(t *testing.T) {
	info := exampleInfo()
	info.MSI.ProductCode = "{12345678-1234-1234-1234-123456789abc}"
	info.MSI.UpgradeCode = "{abcdef01-2345-6789-abcd-ef0123456789}"
	packageAndValidate(t, info)
}

// TestDerivedProductCode guards against shipping an MSI without a ProductCode
// (msiexec fails such installs with error 1605). When the config omits the
// codes, a derived braced GUID must be written into the package.
func TestDerivedProductCode(t *testing.T) {
	info := exampleInfo()
	require.Empty(t, info.MSI.ProductCode)
	require.Empty(t, info.MSI.UpgradeCode)

	var buf bytes.Buffer
	require.NoError(t, msi.Default.Package(info, &buf))

	guid := regexp.MustCompile(`\{[0-9A-F]{8}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{12}\}`)
	require.True(t, guid.Match(buf.Bytes()), "a derived GUID must be present in the package")

	// Derivation must be reproducible.
	var buf2 bytes.Buffer
	require.NoError(t, msi.Default.Package(exampleInfo(), &buf2))
	require.Equal(t, buf.Bytes(), buf2.Bytes(), "builds with identical input must be reproducible")
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
	packageAndValidate(t, info)
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

func TestService(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/Program Files/TestApp/svc.exe",
	})
	info.MSI.Services = []nfpm.MSIService{
		{
			Name:        "TestSvc",
			DisplayName: "Test Service",
			Executable:  "/Program Files/TestApp/svc.exe",
			Description: "A test service",
			StartType:   "auto",
			Start:       true,
			Stop:        true,
		},
	}
	packageAndValidate(t, info)
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
	info := exampleInfo()
	info.MSI.Services = []nfpm.MSIService{
		{Name: "TestSvc", Executable: "/Program Files/TestApp/app.exe", StartType: "bogus"},
	}
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "start_type")
}

func TestRegistry(t *testing.T) {
	info := exampleInfo()
	info.MSI.Registry = []nfpm.MSIRegistry{
		{Root: "HKLM", Key: `Software\TestCo\TestApp`, Name: "InstallPath", Value: "C:\\TestApp"},
		{Root: "HKCU", Key: `Software\TestCo\TestApp`, Name: "Enabled", Value: "1"},
	}
	packageAndValidate(t, info)
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

func TestMajorUpgrade(t *testing.T) {
	info := exampleInfo()
	info.MSI.UpgradeCode = "{ABCDEF01-2345-6789-ABCD-EF0123456789}"
	info.MSI.Upgrade = nfpm.MSIUpgrade{
		Enabled:               true,
		DowngradeErrorMessage: "A newer version is already installed.",
	}
	packageAndValidate(t, info)
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
	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("Test license text")), "license text must be embedded for the UI")
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
	require.NotNil(t, info.MSI.AllUsers)
	require.True(t, *info.MSI.AllUsers)
	require.Equal(t, "demand", info.MSI.Services[0].StartType)
	require.Equal(t, "ProgramMenuFolder", info.MSI.Shortcuts[0].Directory)
}

// TestRootFieldProperties proves shared root metadata lands in the standard
// ARP property rows.
func TestRootFieldProperties(t *testing.T) {
	raw := packageAndValidate(t, exampleInfo())
	require.True(t, bytes.Contains(raw, []byte("ARPCOMMENTS")))
	require.True(t, bytes.Contains(raw, []byte("Test application")))
	require.True(t, bytes.Contains(raw, []byte("ARPURLINFOABOUT")))
	require.True(t, bytes.Contains(raw, []byte("https://example.com")))
}

func TestRootFieldPropertiesUserOverride(t *testing.T) {
	info := exampleInfo()
	info.MSI.Properties = map[string]string{"ARPCOMMENTS": "custom comment"}
	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("custom comment")))
	require.False(t, bytes.Contains(raw, []byte("Test application")),
		"root description must not override the user-provided property")
}

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
	raw := packageAndValidate(t, info)
	for _, id := range []string{"NfpmPreInstall", "NfpmPostInstall", "NfpmPreRemove", "NfpmPostRemove"} {
		require.True(t, bytes.Contains(raw, []byte(id)), "custom action %s must be present", id)
	}
	require.True(t, bytes.Contains(raw, []byte("UPGRADINGPRODUCTCODE")),
		"remove hooks must be conditioned on the major-upgrade guard")
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

func TestScriptTooLarge(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.ps1")
	require.NoError(t, os.WriteFile(big, bytes.Repeat([]byte("# padding\n"), 1000), 0o600))

	info := exampleInfo()
	info.Scripts.PreRemove = big
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
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

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
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
	require.Positive(t, hits, "the configured timestamp URL must actually be requested")
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

	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("ProgramFilesFolder")))
	require.False(t, bytes.Contains(raw, []byte("ProgramFiles64Folder")),
		"a 32-bit package must not reference the 64-bit program files folder")
}

// TestPackageSystemDir proves files installed into a Windows system directory
// are marked permanent, which ICE09 requires.
func TestPackageSystemDir(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/Windows/System32/testapp.dll",
	})
	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("System64Folder")))
}

func TestSymlinkSkipped(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "/Program Files/TestApp/app.exe",
		Destination: "/Program Files/TestApp/link.exe",
		Type:        files.TypeSymlink,
	})
	packageAndValidate(t, info)
	require.Contains(t, logs.String(), "msi does not support symlinks")
}

func TestDirectoryContentSkipped(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Destination: "/Program Files/TestApp/data",
		Type:        files.TypeDir,
	})
	packageAndValidate(t, info)
}

// TestMultipleLicenses proves the first license wins for the install UI and the
// rest are still installed as ordinary files.
func TestMultipleLicenses(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

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

	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("First license text")), "the first license feeds the UI")
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
	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("--verbose")))
	require.True(t, bytes.Contains(raw, []byte("DesktopFolder")))
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

// TestServiceOptionalFields exercises every optional ServiceInstall field, and
// the install-only ServiceControl arm (Start without Stop).
func TestServiceOptionalFields(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/Program Files/TestApp/svc.exe",
	})
	info.MSI.Services = []nfpm.MSIService{
		{
			Name:         "TestSvc",
			DisplayName:  "Test Service",
			Executable:   "/Program Files/TestApp/svc.exe",
			Description:  "A test service",
			StartType:    "auto",
			Account:      "NT AUTHORITY\\LocalService",
			Arguments:    "--serve",
			Dependencies: []string{"Tcpip", "Dnscache"},
			Start:        true,
		},
	}
	raw := packageAndValidate(t, info)
	require.True(t, bytes.Contains(raw, []byte("Test Service")))
	require.True(t, bytes.Contains(raw, []byte("--serve")))
}

// TestServiceStopOnly covers the uninstall-only ServiceControl arm.
func TestServiceStopOnly(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/Program Files/TestApp/svc.exe",
	})
	info.MSI.Services = []nfpm.MSIService{
		{
			Name:       "TestSvc",
			Executable: "/Program Files/TestApp/svc.exe",
			StartType:  "demand",
			Stop:       true,
		},
	}
	packageAndValidate(t, info)
}

// TestServiceNoStartNoStop covers the arm where no ServiceControl row is added.
func TestServiceNoStartNoStop(t *testing.T) {
	info := exampleInfo()
	info.Contents = append(info.Contents, &files.Content{
		Source:      "../testdata/fake",
		Destination: "/Program Files/TestApp/svc.exe",
	})
	info.MSI.Services = []nfpm.MSIService{
		{
			Name:       "TestSvc",
			Executable: "/Program Files/TestApp/svc.exe",
			StartType:  "demand",
		},
	}
	packageAndValidate(t, info)
}

// TestNoRootMetadata covers the arms where the ARP properties are skipped
// because the corresponding root fields are empty.
func TestNoRootMetadata(t *testing.T) {
	info := exampleInfo()
	info.Description = ""
	info.Homepage = ""
	raw := packageAndValidate(t, info)
	require.False(t, bytes.Contains(raw, []byte("ARPCOMMENTS")))
	require.False(t, bytes.Contains(raw, []byte("ARPURLINFOABOUT")))
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
