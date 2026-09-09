// Package maintainer derives publisher-style names from the shared root
// package metadata, for packagers whose formats need a plain organization or
// person name (the Windows packagers).
package maintainer

import "strings"

// Name returns the name part of an RFC 822 style maintainer string
// ("Jane Doe <jane@example.com>" -> "Jane Doe"), trimmed of surrounding
// whitespace. A maintainer without an address is returned as-is.
func Name(maintainer string) string {
	if i := strings.IndexByte(maintainer, '<'); i >= 0 {
		maintainer = maintainer[:i]
	}
	return strings.TrimSpace(maintainer)
}

// VendorOrMaintainer returns vendor when it is set, otherwise the name part of
// maintainer. It is the shared default for fields such as the MSIX publisher
// and the MSI manufacturer.
func VendorOrMaintainer(vendor, maintainer string) string {
	if vendor != "" {
		return vendor
	}
	return Name(maintainer)
}
