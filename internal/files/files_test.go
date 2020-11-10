package files

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goreleaser/nfpm"
)

func TestListFilesToCopy(t *testing.T) {
	info := &nfpm.Info{
		Overridables: nfpm.Overridables{
			ConfigFiles: map[string]string{
				"../../testdata/whatever.conf": "/whatever",
			},
			Files: map[string]string{
				"../../testdata/scripts/*": "/test",
			},
		},
	}

	regularFiles, err := Expand(info.Files, info.DisableGlobbing)
	require.NoError(t, err)

	configFiles, err := Expand(info.ConfigFiles, info.DisableGlobbing)
	require.NoError(t, err)

	// all the input files described in the config in sorted order by source path
	require.Equal(t, []FileToCopy{
		{"../../testdata/scripts/postinstall.sh", "/test/postinstall.sh"},
		{"../../testdata/scripts/postremove.sh", "/test/postremove.sh"},
		{"../../testdata/scripts/preinstall.sh", "/test/preinstall.sh"},
		{"../../testdata/scripts/preremove.sh", "/test/preremove.sh"},
	}, regularFiles)

	require.Equal(t, []FileToCopy{
		{"../../testdata/whatever.conf", "/whatever"},
	}, configFiles)
}

func TestListFilesToCopyWithAndWithoutGlobbing(t *testing.T) {
	_, err := Expand(map[string]string{
		"../../testdata/{file}*": "/test/{file}*",
	}, false)
	assert.EqualError(t, err, "glob failed: ../../testdata/{file}*: no matching files")

	files, err := Expand(map[string]string{
		"../../testdata/{file}*": "/test/{file}*",
	}, true)
	require.NoError(t, err)
	assert.Equal(t, []FileToCopy{
		{"../../testdata/{file}*", "/test/{file}*"},
	}, files)
}
