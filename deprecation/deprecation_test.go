package deprecation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotice(t *testing.T) {
	var b bytes.Buffer
	Noticer = prefixed{&b}
	Print("blah\n")
	Printf("blah: %v\n", true)
	Println("foobar")
	require.Equal(t, "DEPRECATION WARNING: blah\nDEPRECATION WARNING: blah: true\nDEPRECATION WARNING: foobar\n", b.String())
}
