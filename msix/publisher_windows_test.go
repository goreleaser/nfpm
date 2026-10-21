package msix

import (
	"bytes"
	"encoding/asn1"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"unsafe"

	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Compare with the same native API and flags Windows uses to derive an MSIX
// signer's publisher, in the regular Windows unit-test job on every PR.
func TestCertificatePublisherWindows(t *testing.T) {
	certNameToStr := windows.NewLazySystemDLL("crypt32.dll").NewProc("CertNameToStrW")
	const (
		x509ASNEncoding = 1
		x500Reverse     = 3 | 0x02000000
	)
	for _, tt := range publisherSubjects() {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := asn1.Marshal(tt.rdns)
			require.NoError(t, err)
			blob := windows.CertNameBlob{Size: uint32(len(raw)), Data: &raw[0]}
			size, _, callErr := certNameToStr.Call(x509ASNEncoding, uintptr(unsafe.Pointer(&blob)), x500Reverse, 0, 0)
			require.NotZero(t, size, "CertNameToStrW: %v", callErr)
			buf := make([]uint16, size)
			written, _, callErr := certNameToStr.Call(
				x509ASNEncoding, uintptr(unsafe.Pointer(&blob)), x500Reverse, uintptr(unsafe.Pointer(&buf[0])), size,
			)
			require.Equal(t, size, written, "CertNameToStrW: %v", callErr)
			publisher, err := certificatePublisher(raw)
			require.NoError(t, err)
			require.Equal(t, windows.UTF16ToString(buf), publisher)
		})
	}
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
			require.NoError(t, Default.Package(info, &buf))
			root := t.TempDir()
			pkg := filepath.Join(root, "publisher.msix")
			require.NoError(t, os.WriteFile(pkg, buf.Bytes(), 0o600))
			cmd := exec.CommandContext(t.Context(), makeappx, "unpack", "/p", pkg, "/d", filepath.Join(root, "unpacked"), "/o")
			output, err := cmd.CombinedOutput()
			if err == nil {
				cmd = exec.CommandContext(t.Context(), makeappx, "pack", "/d", filepath.Join(root, "unpacked"), "/p", filepath.Join(root, "validated.msix"), "/o")
				output, err = cmd.CombinedOutput()
			}
			if tt.valid {
				require.NoError(t, err, "%s", output)
			} else {
				require.Error(t, err, "MakeAppx must reject the invalid publisher: %s", output)
			}
		})
	}
}
