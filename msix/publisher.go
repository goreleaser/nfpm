package msix

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"strings"
	"unicode/utf8"
)

// certificatePublisher follows CertNameToStrW with CERT_X500_NAME_STR and
// CERT_NAME_STR_REVERSE_FLAG, as used by Windows MSIX signature validation.
// Decode RawSubject so repeated attributes retain their encoded order.
// Multi-valued RDNs cannot satisfy both the manifest schema and Windows'
// certificate comparison, so reject them instead of flattening them.
func certificatePublisher(rawSubject []byte) (string, error) {
	var rdns pkix.RDNSequence
	rest, err := asn1.Unmarshal(rawSubject, &rdns)
	if err != nil {
		return "", err
	}
	if len(rest) != 0 || len(rdns) == 0 {
		return "", fmt.Errorf("invalid certificate subject")
	}
	parts := make([]string, 0, len(rdns))
	for i := len(rdns) - 1; i >= 0; i-- {
		if len(rdns[i]) == 0 {
			return "", fmt.Errorf("empty certificate subject RDN")
		}
		if len(rdns[i]) != 1 {
			return "", fmt.Errorf("multi-valued RDN is not supported by MSIX; use a signing certificate with one attribute per RDN")
		}
		attribute := rdns[i][0]
		value, ok := attribute.Value.(string)
		if !ok {
			return "", fmt.Errorf("unsupported certificate subject value for %s", attribute.Type)
		}
		parts = append(parts, windowsDNKey(attribute.Type)+"="+quoteDNValue(value))
	}
	publisher := strings.Join(parts, ", ")
	if err := validatePublisher(publisher); err != nil {
		return "", err
	}
	return publisher, nil
}

func validatePublisher(publisher string) error {
	if !validXMLText(publisher) || utf8.RuneCountInString(publisher) > 8192 || strings.TrimSpace(publisher) != publisher || !publisherPattern.MatchString(publisher) {
		return fmt.Errorf("package msix.publisher must be a Windows distinguished name of at most 8192 characters, with comma-space separators and quoted reserved characters")
	}
	return nil
}

// encoding/xml replaces invalid characters, which would change the signed
// publisher identity. Reject them before encoding instead.
func validXMLText(value string) bool {
	return utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool {
		return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0xfffe || r == 0xffff
	})
}

// Windows doubles embedded quotes and quotes reserved characters and values
// with leading/trailing whitespace; backslashes are literal, not RFC 2253 escapes.
func quoteDNValue(value string) string {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, ",+=\"\\<>#;\n\r") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func windowsDNKey(oid asn1.ObjectIdentifier) string {
	keys := map[string]string{
		"2.5.4.3":                    "CN",
		"2.5.4.4":                    "SN",
		"2.5.4.5":                    "SERIALNUMBER",
		"2.5.4.6":                    "C",
		"2.5.4.7":                    "L",
		"2.5.4.8":                    "S",
		"2.5.4.9":                    "STREET",
		"2.5.4.10":                   "O",
		"2.5.4.11":                   "OU",
		"2.5.4.12":                   "T",
		"2.5.4.13":                   "Description",
		"2.5.4.17":                   "PostalCode",
		"2.5.4.18":                   "POBox",
		"2.5.4.20":                   "Phone",
		"2.5.4.24":                   "X21Address",
		"2.5.4.42":                   "G",
		"2.5.4.43":                   "I",
		"2.5.4.46":                   "dnQualifier",
		"1.2.840.113549.1.9.1":       "E",
		"0.9.2342.19200300.100.1.25": "DC",
	}
	if key, ok := keys[oid.String()]; ok {
		return key
	}
	return "OID." + oid.String()
}
