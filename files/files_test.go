package files_test

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

func TestSymlinksInDirectory(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: testdata/symlinks/subdir
  dst: /bla
- src: testdata/symlinks/link-1
  dst: /
- src: testdata/symlinks/link-2
  dst: /
- src: existent
  dst: /bla/link-3
  type: symlink
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)

	config.Contents, err = files.ExpandContentGlobs(config.Contents, true)
	require.NoError(t, err)
	require.Len(t, config.Contents, 6)

	// Nulling FileInfo to check equality between expected and result
	for _, c := range config.Contents {
		c.FileInfo = nil
	}

	expected := files.Contents{
		{
			Source:      "testdata/symlinks/subdir/existent",
			Destination: "/bla/existent",
			Type:        "",
		},
		{
			Source:      "non-existent",
			Destination: "/bla/link-1",
			Type:        "symlink",
		},
		{
			Source:      "existent",
			Destination: "/bla/link-2",
			Type:        "symlink",
		},
		{
			Source:      "existent",
			Destination: "/bla/link-3",
			Type:        "symlink",
		},
		{
			Source:      "broken",
			Destination: "/link-1",
			Type:        "symlink",
		},
		{
			Source:      "bla",
			Destination: "/link-2",
			Type:        "symlink",
		},
	}
	require.Equal(t, expected, config.Contents)
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

func TestDisableGlobbing(t *testing.T) {
	testCases := []files.Content{
		{
			Source:      "testdata/{test}/bar",
			Destination: "/etc/{test}/bar",
		},
		{
			Source:      "testdata/{test}/[f]oo",
			Destination: "testdata/{test}/[f]oo",
		},
		{
			Source:      "testdata/globtest/a.txt",
			Destination: "testdata/globtest/a.txt",
		},
		{
			Source:      "testdata/globtest/a.txt",
			Destination: "/etc/a.txt",
		},
	}

	disableGlobbing := true

	for i, testCase := range testCases {
		content := testCase

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			result, err := files.ExpandContentGlobs(files.Contents{&content}, disableGlobbing)
			if err != nil {
				t.Fatalf("expand content globs: %v", err)
			}

			if len(result) != 1 {
				t.Fatalf("unexpected result length: %d, expected one", len(result))
			}

			actualContent := result[0]

			// we expect the result content to be identical to the input content
			if actualContent.Source != content.Source {
				t.Fatalf("unexpected content source: %q, expected %q", actualContent.Source, content.Source)
			}

			if actualContent.Destination != content.Destination {
				t.Fatalf("unexpected content destination: %q, expected %q", actualContent.Destination, content.Destination)
			}
		})
	}
}

func TestGlobbingWhenFilesHaveBrackets(t *testing.T) {
	result, err := files.ExpandContentGlobs(files.Contents{
		{
			Source:      "./testdata/\\{test\\}/",
			Destination: ".",
		},
	}, false)
	if err != nil {
		t.Fatalf("expand content globs: %v", err)
	}

	expected := files.Contents{
		{
			Source:      "testdata/{test}/[f]oo",
			Destination: "[f]oo",
		},
		{
			Source:      "testdata/{test}/bar",
			Destination: "bar",
		},
	}

	if len(result) != 2 {
		t.Fatalf("unexpected result length: %d, expected one", len(result))
	}

	for i, r := range result {
		ex := expected[i]
		if ex.Source != r.Source {
			t.Fatalf("unexpected content source: %q, expected %q", r.Source, ex.Source)
		}
		if ex.Destination != r.Destination {
			t.Fatalf("unexpected content destination: %q, expected %q", r.Destination, ex.Destination)
		}
	}
}

func TestGlobbingFilesWithDifferentSizesWithFileInfo(t *testing.T) {
	result, err := files.ExpandContentGlobs(files.Contents{
		{
			Source:      "./testdata/globtest/different-sizes/**/*",
			Destination: ".",
			FileInfo: &files.ContentFileInfo{
				Mode: 0o777,
			},
		},
	}, false)
	if err != nil {
		t.Fatalf("expand content globs: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("unexpected result length: %d, expected 2", len(result))
	}

	if result[0].FileInfo.Size == result[1].FileInfo.Size {
		t.Fatal("test FileInfos have the same size, expected different")
	}
}
