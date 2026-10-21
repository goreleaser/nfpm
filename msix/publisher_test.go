package msix

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"

	"github.com/stretchr/testify/require"
)

func publisherSubjects() []struct {
	name string
	rdns pkix.RDNSequence
	want string
} {
	return []struct {
		name string
		rdns pkix.RDNSequence
		want string
	}{
		{
			"multi-attribute with province",
			pkix.Name{
				Country: []string{"US"}, Province: []string{"Washington"},
				Organization: []string{"TestCompany"}, CommonName: "TestCompany",
			}.ToRDNSequence(),
			"CN=TestCompany, O=TestCompany, S=Washington, C=US",
		},
		{
			"encoded order and repeated attributes",
			pkix.RDNSequence{
				{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "First"}},
				{{Type: asn1.ObjectIdentifier{2, 5, 4, 10}, Value: "Organization"}},
				{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "Last"}},
			},
			"CN=Last, O=Organization, CN=First",
		},
		{
			"multi-valued RDN grouping",
			pkix.RDNSequence{
				{{Type: asn1.ObjectIdentifier{2, 5, 4, 6}, Value: "US"}},
				{
					{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: "Test"},
					{Type: asn1.ObjectIdentifier{2, 5, 4, 11}, Value: "Team"},
				},
			},
			"CN=Test + OU=Team, C=US",
		},
		{
			"unknown OID",
			pkix.RDNSequence{{{Type: asn1.ObjectIdentifier{1, 2, 3, 4}, Value: "Custom"}}},
			"OID.1.2.3.4=Custom",
		},
		{
			"reserved characters",
			pkix.Name{CommonName: `Test, + = " \ < > # ;`}.ToRDNSequence(),
			`CN="Test, + = "" \ < > # ;"`,
		},
		{
			"edge spaces",
			pkix.Name{CommonName: " Test "}.ToRDNSequence(),
			`CN=" Test "`,
		},
		{
			"empty attribute",
			pkix.RDNSequence{{{Type: asn1.ObjectIdentifier{2, 5, 4, 3}, Value: ""}}},
			`CN=""`,
		},
	}
}

func TestCertificatePublisher(t *testing.T) {
	for _, tt := range publisherSubjects() {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := asn1.Marshal(tt.rdns)
			require.NoError(t, err)
			publisher, err := certificatePublisher(raw)
			require.NoError(t, err)
			require.Equal(t, tt.want, publisher)
		})
	}
}

func TestCertificatePublisherInvalid(t *testing.T) {
	for _, raw := range [][]byte{nil, {0x30, 0}, {0x30, 0, 0}} {
		_, err := certificatePublisher(raw)
		require.Error(t, err)
	}
	raw, err := asn1.Marshal(pkix.RDNSequence{{{Type: asn1.ObjectIdentifier{1, 2, 3, 4}, Value: 123}}})
	require.NoError(t, err)
	_, err = certificatePublisher(raw)
	require.ErrorContains(t, err, "unsupported certificate subject value")
}
