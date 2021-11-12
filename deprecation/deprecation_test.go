package deprecation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotice(t *testing.T) {
	Notice("blah")

	var b bytes.Buffer
	Set(&defaultNoticer{w: &b})
	Notice("blah")
	require.Equal(t, b.String(), "DEPRECATION WARNING: blah\n")
}
