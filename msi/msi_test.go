package msi_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
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

// packageAndValidate builds an MSI and asserts it is a structurally valid,
// ICE-clean Windows Installer database.
func packageAndValidate(t *testing.T, info *nfpm.Info) {
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

func TestNoManufacturer(t *testing.T) {
	info := exampleInfo()
	info.MSI.Manufacturer = ""
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "msi.manufacturer")
}

func TestInvalidProductCode(t *testing.T) {
	info := exampleInfo()
	info.MSI.ProductCode = "not-a-guid"
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "product_code")
}

func TestExplicitGUIDs(t *testing.T) {
	info := exampleInfo()
	info.MSI.ProductCode = "{12345678-1234-1234-1234-123456789ABC}"
	info.MSI.UpgradeCode = "{ABCDEF01-2345-6789-ABCD-EF0123456789}"
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

func TestMinimalUIWithLicense(t *testing.T) {
	dir := t.TempDir()
	license := filepath.Join(dir, "LICENSE.txt")
	require.NoError(t, os.WriteFile(license, []byte("Test license text"), 0o600))

	info := exampleInfo()
	info.MSI.MinimalUI = true
	info.MSI.License = license
	packageAndValidate(t, info)
}

func TestMissingLicenseFile(t *testing.T) {
	info := exampleInfo()
	info.MSI.License = "/does/not/exist.txt"
	var buf bytes.Buffer
	err := msi.Default.Package(info, &buf)
	require.Error(t, err)
	require.Contains(t, err.Error(), "license")
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
	require.Equal(t, "MyApp", info.MSI.InstallDir)
	require.NotNil(t, info.MSI.AllUsers)
	require.True(t, *info.MSI.AllUsers)
	require.Equal(t, "demand", info.MSI.Services[0].StartType)
	require.Equal(t, "ProgramMenuFolder", info.MSI.Shortcuts[0].Directory)
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
