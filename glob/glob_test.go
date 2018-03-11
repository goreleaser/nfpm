package glob

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, "long", lcp1)

	empty := []string{}
	lcp2 := longestCommonPrefix(empty)
	assert.Equal(t, "", lcp2)

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
	assert.Equal(t, "", lcp3)
}

func TestGlob(t *testing.T) {
	files, err := Glob("testdata/dir_a/dir_*/*", "/foo/bar")
	assert.NoError(t, err)
	assert.Len(t, files, 2)
	assert.Equal(t, "/foo/bar/dir_b/test_b.txt", files["testdata/dir_a/dir_b/test_b.txt"])
	assert.Equal(t, "/foo/bar/dir_c/test_c.txt", files["testdata/dir_a/dir_c/test_c.txt"])

	nilvalue, err := Glob("does/not/exist", "/foo/bar")
	assert.Error(t, err)
	assert.Nil(t, nilvalue)

	nomatches, err := Glob("testdata/nothing*", "/foo/bar")
	assert.Nil(t, nomatches)
	assert.Error(t, err)
}

func TestSingleGlob(t *testing.T) {
	files, err := Glob("testdata/dir_a/dir_b/test_b.txt", "/foo/bar/dest.dat")
	assert.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Equal(t, "/foo/bar/dest.dat", files["testdata/dir_a/dir_b/test_b.txt"])
}
