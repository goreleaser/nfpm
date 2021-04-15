package files_test

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/goreleaser/nfpm/v2/files"
)

type testStruct struct {
	Contents files.Contents `yaml:"contents"`
}

func TestBasicDecode(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: a
  dst: b
- src: a
  dst: b
  type: "config|noreplace"
  packager: "rpm"
  file_info:
    mode: 0644
    mtime: 2008-01-02T15:04:05Z
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	require.Len(t, config.Contents, 2)
	for _, f := range config.Contents {
		t.Logf("%+#v\n", f)
		require.Equal(t, f.Source, "a")
		require.Equal(t, f.Destination, "b")
	}
}

func TestDeepPathsWithGlob(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: testdata/globtest/**/*
  dst: /bla
  file_info:
    mode: 0644
    mtime: 2008-01-02T15:04:05Z
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	require.Len(t, config.Contents, 1)
	parsedContents, err := files.ExpandContentGlobs(config.Contents, false)
	require.NoError(t, err)
	for _, f := range parsedContents {
		switch f.Source {
		case "testdata/globtest/nested/b.txt":
			require.Equal(t, "/bla/nested/b.txt", f.Destination)
		case "testdata/globtest/multi-nested/subdir/c.txt":
			require.Equal(t, "/bla/multi-nested/subdir/c.txt", f.Destination)
		}
	}
}

func TestDeepPathsWithoutGlob(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: testdata/deep-paths/
  dst: /bla
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	require.Len(t, config.Contents, 1)
	parsedContents, err := files.ExpandContentGlobs(config.Contents, true)
	require.NoError(t, err)
	for _, f := range parsedContents {
		switch f.Source {
		case "testdata/deep-paths/nested1/nested2/a.txt":
			require.Equal(t, "/bla/nested1/nested2/a.txt", f.Destination)
		default:
			t.Errorf("unknown source %s", f.Source)
		}
	}
}

func TestFileInfoDefault(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: files_test.go
  dst: b
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)

	config.Contents, err = files.ExpandContentGlobs(config.Contents, true)
	require.NoError(t, err)
	require.Len(t, config.Contents, 1)

	fi, err := os.Stat("files_test.go")
	require.NoError(t, err)

	f := config.Contents[0]
	require.Equal(t, f.Source, "files_test.go")
	require.Equal(t, f.Destination, "b")
	require.Equal(t, f.FileInfo.Mode, fi.Mode())
	require.Equal(t, f.FileInfo.MTime, fi.ModTime())
}

func TestFileInfo(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: files_test.go
  dst: b
  type: "config|noreplace"
  packager: "rpm"
  file_info:
    mode: 0123
    mtime: 2008-01-02T15:04:05Z
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)

	config.Contents, err = files.ExpandContentGlobs(config.Contents, true)
	require.NoError(t, err)
	require.Len(t, config.Contents, 1)

	ct, err := time.Parse(time.RFC3339, "2008-01-02T15:04:05Z")
	require.NoError(t, err)

	f := config.Contents[0]
	require.Equal(t, f.Source, "files_test.go")
	require.Equal(t, f.Destination, "b")
	require.Equal(t, f.FileInfo.Mode, os.FileMode(0o123))
	require.Equal(t, f.FileInfo.MTime, ct)
}

func TestRace(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: a
  dst: b
  type: symlink
`))
	err := dec.Decode(&config)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := files.ExpandContentGlobs(config.Contents, false)
			require.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestCollision(t *testing.T) {
	t.Run("collision between files for all packagers", func(t *testing.T) {
		configuredFiles := []*files.Content{
			{Source: "../testdata/whatever.conf", Destination: "/samedestination"},
			{Source: "../testdata/whatever2.conf", Destination: "/samedestination"},
		}

		_, err := files.ExpandContentGlobs(configuredFiles, true)
		require.ErrorIs(t, err, files.ErrContentCollision)
	})

	t.Run("no collision due to different per-file packagers", func(t *testing.T) {
		configuredFiles := []*files.Content{
			{Source: "../testdata/whatever.conf", Destination: "/samedestination", Packager: "foo"},
			{Source: "../testdata/whatever2.conf", Destination: "/samedestination", Packager: "bar"},
		}

		_, err := files.ExpandContentGlobs(configuredFiles, true)
		require.NoError(t, err)
	})

	t.Run("collision between file for all packagers and file for specific packager", func(t *testing.T) {
		configuredFiles := []*files.Content{
			{Source: "../testdata/whatever.conf", Destination: "/samedestination", Packager: "foo"},
			{Source: "../testdata/whatever2.conf", Destination: "/samedestination", Packager: ""},
		}

		_, err := files.ExpandContentGlobs(configuredFiles, true)
		require.ErrorIs(t, err, files.ErrContentCollision)
	})
}
