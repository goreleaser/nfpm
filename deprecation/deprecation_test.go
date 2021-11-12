package deprecation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotice(t *testing.T) {
	Print("blah")

	var b bytes.Buffer
	Noticer = prefixed{&b}
	Printf("blah: %v\n", true)
	require.Equal(t, b.String(), "DEPRECATION WARNING: blah: true\n")
}
