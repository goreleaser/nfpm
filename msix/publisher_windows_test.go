package msix

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"unsafe"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
	gomsix "go.digitalxero.dev/go-msix"
	"golang.org/x/sys/windows"
)

// Compare with the same native API and flags Windows uses to derive an MSIX
// signer's publisher, in the regular Windows unit-test job on every PR.
func TestCertificatePublisherWindows(t *testing.T) {
	for _, tt := range publisherSubjects() {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := asn1.Marshal(tt.rdns)
			require.NoError(t, err)
			publisher, err := certificatePublisher(raw)
			require.NoError(t, err)
			require.Equal(t, windowsCertificatePublisher(t, raw), publisher)
		})
	}
}

func windowsCertificatePublisher(t *testing.T, raw []byte) string {
	t.Helper()
	certNameToStr := windows.NewLazySystemDLL("crypt32.dll").NewProc("CertNameToStrW")
	const (
		x509ASNEncoding = 1
		x500Reverse     = 3 | 0x02000000
	)
	blob := windows.CertNameBlob{Size: uint32(len(raw)), Data: &raw[0]}
	size, _, callErr := certNameToStr.Call(x509ASNEncoding, uintptr(unsafe.Pointer(&blob)), x500Reverse, 0, 0)
	require.NotZero(t, size, "CertNameToStrW: %v", callErr)
	buf := make([]uint16, size)
	written, _, callErr := certNameToStr.Call(
		x509ASNEncoding, uintptr(unsafe.Pointer(&blob)), x500Reverse, uintptr(unsafe.Pointer(&buf[0])), size,
	)
	require.Equal(t, size, written, "CertNameToStrW: %v", callErr)
	return windows.UTF16ToString(buf)
}

// MakeAppx validates the generated manifest against the Windows SDK schema.
// Include a negative control so accidentally disabling validation cannot pass.
func TestPublisherManifestWindows(t *testing.T) {
	tools, err := filepath.Glob(filepath.Join(os.Getenv("ProgramFiles(x86)"), "Windows Kits", "10", "bin", "*", "x64", "makeappx.exe"))
	require.NoError(t, err)
	if len(tools) == 0 {
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			t.Fatal("Windows SDK makeappx.exe is required on the CI runner")
		}
		t.Skip("Windows SDK makeappx.exe is not installed")
	}
	sort.Strings(tools)
	makeappx := tools[len(tools)-1]
	for _, tt := range []struct {
		name   string
		vendor string
		valid  bool
	}{
		{"comma", "TestCo, Inc", true},
		{"plus", "Test+Co", true},
		{"quote", `Test"Co`, true},
		{"angle brackets", "Test<Co>", true},
		{"hash", "Test#Co", true},
		{"semicolon", "Test;Co", true},
		{"padded vendor", " TestCo ", true},
		{"blank vendor", " \t ", true},
		{"invalid RFC 2253 control", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := exampleInfo()
			info.MSIX.Publisher = ""
			info.Vendor = tt.vendor
			info.MSIX.Properties.Logo = "Assets/logo.png"
			info.Contents = files.Contents{
				{Source: "../testdata/fake", Destination: "/app/fake.exe"},
				{Source: "../testdata/acceptance/testapp/logo.png", Destination: "/Assets/logo.png"},
			}
			if !tt.valid {
				info.MSIX.Publisher = `CN=TestCo\, Inc`
			}
			var buf bytes.Buffer
			if tt.valid {
				require.NoError(t, Default.Package(info, &buf))
			} else {
				require.ErrorContains(t, Default.Package(info, io.Discard), "msix.publisher")
				// Bypass nfpm's validation to retain an independent SDK negative control.
				buf.Write(packageForManifestValidation(t, info))
			}
			checkManifestWindows(t, makeappx, buf.Bytes(), tt.valid)
		})
	}
	t.Run("signed single-valued subject control", func(t *testing.T) {
		pfx, passphrase := makeTestPFX(t)
		info := exampleInfo()
		info.MSIX.Publisher = ""
		info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: passphrase}
		info.MSIX.Properties.Logo = "Assets/logo.png"
		info.Contents = append(info.Contents, &files.Content{
			Source: "../testdata/acceptance/testapp/logo.png", Destination: "/Assets/logo.png",
		})
		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		checkManifestWindows(t, makeappx, buf.Bytes(), true)
	})
	t.Run("signed multi-valued subject", func(t *testing.T) {
		raw, err := asn1.Marshal(pkix.RDNSequence{{
			{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "Test"},
			{Type: asn1.ObjectIdentifier{2, 5, 4, 11}, Value: "Team"},
		}})
		require.NoError(t, err)
		pfx, passphrase := makeTestPFXWithSubject(t, raw, "test123")
		info := exampleInfo()
		info.MSIX.Publisher = ""
		info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: passphrase}
		require.ErrorContains(t, Default.Package(info, io.Discard), "multi-valued RDN")
		info.MSIX.Publisher = windowsCertificatePublisher(t, raw)
		require.Contains(t, info.MSIX.Publisher, " + ")
		info.MSIX.Properties.Logo = "Assets/logo.png"
		info.Contents = append(info.Contents, &files.Content{
			Source: "../testdata/acceptance/testapp/logo.png", Destination: "/Assets/logo.png",
		})
		pkg := packageForManifestValidation(t, info)
		archive, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
		require.NoError(t, err)
		signature, err := archive.Open("AppxSignature.p7x")
		require.NoError(t, err)
		require.NoError(t, signature.Close())
		checkManifestWindows(t, makeappx, pkg, false)
	})
}

func checkManifestWindows(t *testing.T, makeappx string, pkgBytes []byte, valid bool) {
	t.Helper()
	root := t.TempDir()
	pkg := filepath.Join(root, "publisher.msix")
	require.NoError(t, os.WriteFile(pkg, pkgBytes, 0o600))
	cmd := exec.CommandContext(t.Context(), makeappx, "unpack", "/p", pkg, "/d", filepath.Join(root, "unpacked"), "/o")
	output, err := cmd.CombinedOutput()
	if err == nil {
		cmd = exec.CommandContext(t.Context(), makeappx, "pack", "/d", filepath.Join(root, "unpacked"), "/p", filepath.Join(root, "validated.msix"), "/o")
		output, err = cmd.CombinedOutput()
	}
	if valid {
		require.NoError(t, err, "%s", output)
	} else {
		require.Error(t, err, "MakeAppx must reject the invalid publisher: %s", output)
		require.Contains(t, string(output), "Publisher", "failure must concern publisher validation")
	}
}

// Construct deliberately invalid manifests using the lower-level library, so
// native SDK negative controls still run after nfpm learns to reject them.
func packageForManifestValidation(t *testing.T, info *nfpm.Info) []byte {
	t.Helper()
	Default.SetPackagerDefaults(info)
	info = ensureValidArch(info)
	require.NoError(t, nfpm.PrepareForPackager(info, packagerName))
	builder := gomsix.NewBuilder().
		WithIdentity(gomsix.NewIdentity().WithName(info.Name).WithVersion("1.0.0.0").
			WithPublisher(info.MSIX.Publisher).WithProcessorArchitecture(info.Arch).Build()).
		WithProperties(buildProperties(info)).WithDependencies(buildDependencies(info)).
		WithCapabilities(buildCapabilities(info)).AddResource(gomsix.NewResource().WithLanguage("en-us").Build())
	for _, app := range buildApplications(info) {
		builder.AddApplication(app)
	}
	addContents(builder, info)
	if info.MSIX.Signature.PFXFile != "" {
		cert, key, chain, err := gomsix.LoadPFX(info.MSIX.Signature.PFXFile, info.MSIX.Signature.KeyPassphrase)
		require.NoError(t, err)
		builder.WithSigning(gomsix.NewSigning().WithCertificate(cert).WithPrivateKey(key).WithCertChain(chain...).Build())
	}
	var buf bytes.Buffer
	require.NoError(t, builder.Build(context.Background(), &buf))
	return buf.Bytes()
}
