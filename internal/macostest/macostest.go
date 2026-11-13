// Package macostest holds the fixtures the tests of the macOS packagers
// (app, dmg and pkg) share.
package macostest

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"debug/macho"
	"encoding/asn1"
	"encoding/binary"
	"io/fs"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

// Passphrase protects the identities made by Identity.
const Passphrase = "test123"

// The certificate extensions Apple marks its Developer ID certificates with.
// nolint: gochecknoglobals
var (
	DeveloperIDApplication = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 1, 13}
	DeveloperIDInstaller   = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 1, 14}
)

// MTime is a fixed package time, so tests build reproducible output.
// nolint: gochecknoglobals
var MTime = time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)

// WriteFile creates a source file for a contents entry and returns its path.
func WriteFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// CaptureLogs redirects the warnings the packagers log to the returned
// buffer for the rest of the test.
func CaptureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &logs
}

// Identity creates a throwaway self-signed signing identity, optionally
// carrying Developer ID marker extensions, and returns the path of its
// PKCS#12 file and the certificate.
func Identity(t *testing.T, markers ...asn1.ObjectIdentifier) (string, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "nfpm test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	for _, marker := range markers {
		template.ExtraExtensions = append(template.ExtraExtensions,
			pkix.Extension{Id: marker, Value: []byte{0x05, 0x00}})
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	p12, err := pkcs12.Modern.Encode(key, certificate, nil, Passphrase)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "identity.p12")
	require.NoError(t, os.WriteFile(path, p12, 0o600))
	return path, certificate
}

// Program builds the acceptance test program for macOS on the given Go
// architecture (arm64 or amd64) and returns the path of the Mach-O binary.
// The Go linker signs it ad hoc, as it does every darwin binary.
func Program(t *testing.T, goarch string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "program-"+goarch)
	cmd := exec.Command("go", "build", "-o", out, "github.com/goreleaser/nfpm/v2/testdata/acceptance/testapp")
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	return out
}

// UniversalProgram returns a universal (fat) Mach-O binary holding the test
// program for both architectures, laid out as lipo does.
func UniversalProgram(t *testing.T) string {
	t.Helper()
	const (
		fatMagic  = 0xcafebabe
		cpuAMD64  = 0x01000007
		cpuARM64  = 0x0100000c
		alignment = 14 // 16 KiB, the page size lipo aligns slices to
	)
	slices := []struct {
		cpu, sub uint32
		body     []byte
	}{
		{cpuAMD64, 3, readFile(t, Program(t, "amd64"))},
		{cpuARM64, 0, readFile(t, Program(t, "arm64"))},
	}

	var fat bytes.Buffer
	write := func(values ...uint32) {
		for _, value := range values {
			require.NoError(t, binary.Write(&fat, binary.BigEndian, value))
		}
	}
	write(fatMagic, uint32(len(slices)))
	offset := uint32(1) << alignment
	offsets := make([]uint32, len(slices))
	for n, slice := range slices {
		offsets[n] = offset
		write(slice.cpu, slice.sub, offset, uint32(len(slice.body)), alignment)
		offset += (uint32(len(slice.body)) + 1<<alignment - 1) &^ (1<<alignment - 1)
	}
	for n, slice := range slices {
		fat.Write(make([]byte, int(offsets[n])-fat.Len()))
		fat.Write(slice.body)
	}

	path := filepath.Join(t.TempDir(), "program-universal")
	require.NoError(t, os.WriteFile(path, fat.Bytes(), 0o755))
	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return body
}

// CodeSignature returns the code signature embedded in a thin Mach-O binary,
// or nil when it has none. A signature made with an identity contains that
// identity's certificate; an ad hoc one, like the Go linker's, contains none.
func CodeSignature(t *testing.T, binaryFile []byte) []byte {
	t.Helper()
	const loadCodeSignature = 0x1d
	file, err := macho.NewFile(bytes.NewReader(binaryFile))
	require.NoError(t, err)
	for _, load := range file.Loads {
		raw := load.Raw()
		if file.ByteOrder.Uint32(raw) != loadCodeSignature {
			continue
		}
		offset := file.ByteOrder.Uint32(raw[8:])
		size := file.ByteOrder.Uint32(raw[12:])
		require.LessOrEqual(t, int(offset+size), len(binaryFile))
		return binaryFile[offset : offset+size]
	}
	return nil
}

// ReadFile returns the content of a regular file in a file tree.
func ReadFile(t *testing.T, fsys fs.FS, name string) []byte {
	t.Helper()
	body, err := fs.ReadFile(fsys, name)
	require.NoError(t, err)
	return body
}

// Lstat returns the entry's own info, without following a symbolic link.
func Lstat(t *testing.T, fsys fs.FS, name string) fs.FileInfo {
	t.Helper()
	if links, ok := fsys.(fs.ReadLinkFS); ok {
		info, err := links.Lstat(name)
		require.NoError(t, err)
		return info
	}
	info, err := fs.Stat(fsys, name)
	require.NoError(t, err)
	return info
}

// ReadLink returns the target of a symbolic link in a file tree. A zip
// archive stores the target as the entry's content.
func ReadLink(t *testing.T, fsys fs.FS, name string) string {
	t.Helper()
	require.Equal(t, fs.ModeSymlink, Lstat(t, fsys, name).Mode().Type(), "%s is not a symbolic link", name)
	if links, ok := fsys.(fs.ReadLinkFS); ok {
		target, err := links.ReadLink(name)
		require.NoError(t, err)
		return target
	}
	return string(ReadFile(t, fsys, name))
}
