package files

import (
	"testing"

	"github.com/goreleaser/nfpm"
	"github.com/stretchr/testify/require"
)

func TestListFilesToCopy(t *testing.T) {
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			ConfigFiles: map[string]string{
				"../../testdata/whatever.conf": "/whatever",
			},
			Files: map[string]string{
				"../../testdata/scripts/**/*": "/test",
			},
		},
	}

	files, err := FilesToCopy(info)
	require.NoError(t, err)

	// all the input files described in the config in sorted order by source path
	require.Equal(t, []FileToCopy{
		{
			"../../testdata/scripts/postinstall.sh",
			"/test/postinstall.sh",
			false,
		},
		{
			"../../testdata/scripts/postremove.sh",
			"/test/postremove.sh",
			false,
		},
		{
			"../../testdata/scripts/preinstall.sh",
			"/test/preinstall.sh",
			false,
		},
		{
			"../../testdata/scripts/preremove.sh",
			"/test/preremove.sh",
			false,
		},
		{
			"../../testdata/whatever.conf",
			"/whatever",
			true,
		},
	}, files)
}
