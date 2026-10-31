package msix

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublisherAfterSigningRetry(t *testing.T) {
	pfx, passphrase := makeTestPFX(t)
	info := exampleInfo()
	info.MSIX.Publisher = ""
	info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: "wrong"}
	var signingErr *nfpm.ErrSigningFailure
	require.ErrorAs(t, Default.Package(info, io.Discard), &signingErr)
	assert.Empty(t, info.MSIX.Publisher, "failed attempts must not persist a derived identity")
	info.MSIX.Signature.KeyPassphrase = passphrase
	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))
	assert.Equal(t, `CN="nfpm, msix", O=TestCompany, S=Washington, C=US`, decodedPublisher(t, buf.Bytes()))
	assert.Empty(t, info.MSIX.Publisher)
}

func TestPublisherReusedInfo(t *testing.T) {
	t.Run("changed signing certificate", func(t *testing.T) {
		pfx, passphrase := makeTestPFX(t)
		info := exampleInfo()
		info.MSIX.Publisher = ""
		info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: passphrase}
		require.NoError(t, Default.Package(info, io.Discard))
		raw, err := asn1.Marshal(pkix.Name{CommonName: "Other Company"}.ToRDNSequence())
		require.NoError(t, err)
		info.MSIX.Signature.PFXFile, _ = makeTestPFXWithSubject(t, raw, passphrase)
		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		assert.Equal(t, "CN=Other Company", decodedPublisher(t, buf.Bytes()))
	})
	t.Run("changed unsigned vendor", func(t *testing.T) {
		info := exampleInfo()
		info.MSIX.Publisher = ""
		require.NoError(t, Default.Package(info, io.Discard))
		info.Vendor = "Other Company"
		var buf bytes.Buffer
		require.NoError(t, Default.Package(info, &buf))
		assert.Equal(t, "CN=Other Company", decodedPublisher(t, buf.Bytes()))
	})
}

func TestSigningPublisherMismatch(t *testing.T) {
	pfx, passphrase := makeTestPFX(t)
	info := exampleInfo()
	info.MSIX.Publisher = "CN=Different Company"
	info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: passphrase}
	var signingErr *nfpm.ErrSigningFailure
	require.ErrorAs(t, Default.Package(info, io.Discard), &signingErr)
	assert.Contains(t, signingErr.Error(), "does not match")
}

func TestPublisherDisplayNameMetadata(t *testing.T) {
	for _, tt := range []struct {
		name, vendor, maintainer, want string
	}{
		{"padded vendor", " TestCo ", "", "TestCo"},
		{"blank vendor", " \t ", "", "MyCompany.TestApp"},
		{"blank vendor with maintainer", " \t ", "Jane Doe <jane@example.com>", "Jane Doe"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := exampleInfo()
			info.Vendor, info.Maintainer = tt.vendor, tt.maintainer
			var buf bytes.Buffer
			require.NoError(t, Default.Package(info, &buf))
			var manifest struct {
				PublisherDisplayName string `xml:"Properties>PublisherDisplayName"`
			}
			require.NoError(t, xml.Unmarshal([]byte(readManifest(t, buf.Bytes())), &manifest))
			assert.Equal(t, tt.want, manifest.PublisherDisplayName)
		})
	}
}

func TestPublisherMetadataLimits(t *testing.T) {
	for _, tt := range []struct {
		name, publisher, vendor, wantError string
	}{
		{"long derived display name", "CN=Test", strings.Repeat("x", 257), "publisher_display_name"},
		{"long publisher", "CN=" + strings.Repeat("x", 8190), "Test", "publisher"},
		{"malformed publisher", "CN=Test + OU=Team", "Test", "publisher"},
		{"line break in quoted publisher", "CN=\"Test\nCompany\"", "Test", "publisher"},
		{"quoted multi-valued publisher", `CN="Test" + OU="Team"`, "Test", "publisher"},
		{"XML control in publisher", "CN=\"Test\x00Company\"", "Test", "publisher"},
		{"XML control in derived display name", "CN=Test", "Test\x00Company", "publisher_display_name"},
		{"line break in derived display name", "CN=Test", "Test\nCompany", "publisher_display_name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := exampleInfo()
			info.MSIX.Publisher, info.Vendor = tt.publisher, tt.vendor
			require.ErrorContains(t, Default.Package(info, io.Discard), tt.wantError)
		})
	}
}

func TestPublisherMetadataBoundaries(t *testing.T) {
	for _, name := range []string{strings.Repeat("x", 256), "ms-resource:" + strings.Repeat("x", 256)} {
		info := exampleInfo()
		info.MSIX.Publisher = "CN=" + strings.Repeat("x", 8189)
		info.MSIX.Properties.PublisherDisplayName = name
		require.NoError(t, Default.Package(info, io.Discard))
	}
}

func TestExplicitPublisherDisplayNameInvalid(t *testing.T) {
	for _, name := range []string{" Test ", " \t ", strings.Repeat("x", 257), "Test\nCompany"} {
		info := exampleInfo()
		info.MSIX.Properties.PublisherDisplayName = name
		require.ErrorContains(t, Default.Package(info, io.Discard), "publisher_display_name")
	}
}

func TestSigningPasswordlessPFX(t *testing.T) {
	t.Setenv("NFPM_PASSPHRASE", "other-packager-password")
	t.Setenv("NFPM_MSIX_PASSPHRASE", "")
	config, err := nfpm.Parse(strings.NewReader("name: TestApp"))
	require.NoError(t, err)
	assert.Empty(t, config.MSIX.Signature.KeyPassphrase)
	pfx, _ := makeTestPFXWithSubject(t, nil, "")
	info := exampleInfo()
	info.MSIX.Publisher = ""
	info.MSIX.Signature = config.MSIX.Signature
	info.MSIX.Signature.PFXFile = pfx
	require.NoError(t, Default.Package(info, io.Discard))
}

func TestSigningMultiValuedSubject(t *testing.T) {
	raw, err := asn1.Marshal(pkix.RDNSequence{{
		{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "Test"},
		{Type: asn1.ObjectIdentifier{2, 5, 4, 11}, Value: "Team"},
	}})
	require.NoError(t, err)
	pfx, passphrase := makeTestPFXWithSubject(t, raw, "test123")
	info := exampleInfo()
	info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: passphrase}
	for _, publisher := range []string{"", "CN=Test + OU=Team"} {
		info.MSIX.Publisher = publisher
		require.ErrorContains(t, Default.Package(info, io.Discard), "multi-valued RDN")
	}
}

func TestSigningSubjectInvalidForManifest(t *testing.T) {
	for _, name := range []string{"Test\nCompany", strings.Repeat("x", 8190)} {
		raw, err := asn1.Marshal(pkix.Name{CommonName: name}.ToRDNSequence())
		require.NoError(t, err)
		pfx, passphrase := makeTestPFXWithSubject(t, raw, "test123")
		info := exampleInfo()
		info.MSIX.Publisher = ""
		info.MSIX.Signature = nfpm.MSIXSignature{PFXFile: pfx, KeyPassphrase: passphrase}
		var signingErr *nfpm.ErrSigningFailure
		require.ErrorAs(t, Default.Package(info, io.Discard), &signingErr)
		assert.Contains(t, signingErr.Error(), "format signing certificate publisher")
		assert.Contains(t, signingErr.Error(), "msix.publisher")
	}
}
