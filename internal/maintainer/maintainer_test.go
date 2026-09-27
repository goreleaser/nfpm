package maintainer_test

import (
	"testing"

	"github.com/goreleaser/nfpm/v2/internal/maintainer"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"Jane Doe <jane@example.com>": "Jane Doe",
		"Jane Doe":                    "Jane Doe",
		"  Jane Doe  ":                "Jane Doe",
		"<jane@example.com>":          "",
		"":                            "",
	} {
		t.Run(in, func(t *testing.T) {
			require.Equal(t, want, maintainer.Name(in))
		})
	}
}

func TestVendorOrMaintainer(t *testing.T) {
	require.Equal(t, "ACME", maintainer.VendorOrMaintainer("ACME", "Jane Doe <jane@example.com>"))
	require.Equal(t, "Jane Doe", maintainer.VendorOrMaintainer("", "Jane Doe <jane@example.com>"))
	require.Empty(t, maintainer.VendorOrMaintainer("", ""))
}
