package pkgutil

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

	lcp1 := LongestCommonPrefix(strings)
	assert.Equal(t, "long", lcp1)

	empty := []string{}
	lcp2 := LongestCommonPrefix(empty)
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

	lcp3 := LongestCommonPrefix(unique)
	assert.Equal(t, "", lcp3)
}

func TestGlob(t *testing.T) {
	files, err := Glob("./testdata/**/*", "/foo/bar")
	assert.NoError(t, err)
	assert.Len(t, files, 2)
	assert.Equal(t, "/foo/bar/b/test_b.txt", files["testdata/a/b/test_b.txt"])
	assert.Equal(t, "/foo/bar/c/test_c.txt", files["testdata/a/c/test_c.txt"])

	nilvalue, err := Glob("./does/not/exist", "/var/lib")
	assert.Error(t, err)
	assert.Nil(t, nilvalue)

	nomatches, err := Glob("./testdata/nothing*", "/foo/bar")
	assert.Nil(t, nomatches)
	assert.Error(t, err)
}
