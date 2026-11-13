package macos

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
)

// timestampNone is the timestamp_url value that signs without a secure
// timestamp, mirroring "codesign --timestamp=none".
const timestampNone = "none"

// CertificateKind identifies the kind of Apple Developer ID certificate a
// format has to be signed with to pass Gatekeeper.
type CertificateKind struct {
	// Name is the kind's name as Apple shows it.
	Name string
	// marker is the certificate extension Apple puts on this kind only.
	marker asn1.ObjectIdentifier
}

// nolint: gochecknoglobals
var (
	// DeveloperIDApplication signs applications and disk images.
	DeveloperIDApplication = CertificateKind{
		Name:   "Developer ID Application",
		marker: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 1, 13},
	}
	// DeveloperIDInstaller signs installer packages.
	DeveloperIDInstaller = CertificateKind{
		Name:   "Developer ID Installer",
		marker: asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 1, 14},
	}
)

// ReadP12 reads a PKCS#12 file named by a signature setting.
func ReadP12(path string) ([]byte, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("p12 file not found: %w", err)
		}
		return nil, fmt.Errorf("unable to access p12 file: %w", err)
	}
	return data, nil
}

// TimestampURL interprets a timestamp_url setting. When override is false the
// signer keeps its default, Apple's timestamp service; otherwise url replaces
// it, and an empty url disables timestamping.
func TimestampURL(setting string) (url string, override bool) {
	switch setting {
	case "":
		return "", false
	case timestampNone:
		return "", true
	default:
		return setting, true
	}
}

// WarnCertificateKind warns when the leaf of a signing chain is not of the
// kind macOS requires. The result is still signed, which is what a test of
// the signing itself needs, so this is not an error.
func WarnCertificateKind(packager, path string, chain [][]byte, kind CertificateKind) {
	if len(chain) == 0 {
		return
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return
	}
	for _, extension := range leaf.Extensions {
		if extension.Id.Equal(kind.marker) {
			return
		}
	}
	log.Printf(
		"warning: %s: the certificate in %s is not a %s certificate, so macOS will not accept the result",
		packager, path, kind.Name)
}
