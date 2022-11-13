package glob

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLongestCommonPrefix(t *testing.T) {
	strings := []string{
		"longestcommonprefix",
		"longestcommontest",
		"longtest",
		"longestcommon",
		"longtest",
		"longesttest",
	}

	lcp1 := longestCommonPrefix(strings)
	require.Equal(t, "long", lcp1)

	empty := []string{}
	lcp2 := longestCommonPrefix(empty)
	require.Equal(t, "", lcp2)

	unique := []string{
		"every",
		"string",
		"is",
		"different",
		"than",
		"one",
		"another",
	}

	lcp3 := longestCommonPrefix(unique)
	require.Equal(t, "", lcp3)
}

func TestGlob(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		files, err := Glob("./testdata/dir_a/dir_*/*", "/foo/bar", false)
		require.NoError(t, err)
		require.Len(t, files, 2)
		require.Equal(t, "/foo/bar/dir_b/test_b.txt", files["testdata/dir_a/dir_b/test_b.txt"])
		require.Equal(t, "/foo/bar/dir_c/test_c.txt", files["testdata/dir_a/dir_c/test_c.txt"])
	})

	t.Run("dst is a dir", func(t *testing.T) {
		files, err := Glob("./testdata/dir_a/dir_b/test_b.txt", "/foo/bar/", false)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "/foo/bar/test_b.txt", files["testdata/dir_a/dir_b/test_b.txt"])
	})

	t.Run("to parent", func(t *testing.T) {
		pattern := "../../testdata/fake"
		abs, err := filepath.Abs(pattern)
		require.NoError(t, err)
		files, err := Glob(pattern, "/foo/fake", false)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "/foo/fake", files[filepath.ToSlash(abs)])
	})

	t.Run("single file", func(t *testing.T) {
		files, err := Glob("testdata/dir_a/dir_b/*", "/foo/bar", false)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "/foo/bar/test_b.txt", files["testdata/dir_a/dir_b/test_b.txt"])
	})

	t.Run("double star", func(t *testing.T) {
		files, err := Glob("testdata/**/test*.txt", "/foo/bar", false)
		require.NoError(t, err)
		require.Len(t, files, 3)
		require.Equal(t, "/foo/bar/dir_a/dir_b/test_b.txt", files["testdata/dir_a/dir_b/test_b.txt"])
	})

	t.Run("nil value", func(t *testing.T) {
		files, err := Glob("does/not/exist", "/foo/bar", false)
		require.EqualError(t, err, "matching \"./does/not/exist\": file does not exist")
		require.Nil(t, files)
	})

	t.Run("no matches", func(t *testing.T) {
		files, err := Glob("testdata/nothing*", "/foo/bar", false)
		require.Nil(t, files)
		require.EqualError(t, err, "glob failed: testdata/nothing*: no matching files")
	})

	t.Run("escaped brace", func(t *testing.T) {
		files, err := Glob("testdata/\\{dir_d\\}/*", "/foo/bar", false)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "/foo/bar/test_brace.txt", files["testdata/{dir_d}/test_brace.txt"])
	})

	t.Run("no glob", func(t *testing.T) {
		files, err := Glob("testdata/dir_a/dir_b/test_b.txt", "/foo/bar/dest.dat", false)
		require.NoError(t, err)
		require.Len(t, files, 1)
		require.Equal(t, "/foo/bar/dest.dat", files["testdata/dir_a/dir_b/test_b.txt"])
	})
}
