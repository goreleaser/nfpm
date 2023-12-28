package files_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var mtime = time.Date(2023, 11, 5, 23, 15, 17, 0, time.UTC)

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
		require.Equal(t, "a", f.Source)
		require.Equal(t, "b", f.Destination)
	}
}

func TestDeepPathsWithGlobAndUmask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foo", "bar", "zaz", "file.txt")
	// create a bunch of files with bad permissions
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o777))
	require.NoError(t, os.WriteFile(path, nil, 0o777))
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: testdata/globtest/**/*
  dst: /bla
  file_info:
    mode: 0644
    mtime: 2008-01-02T15:04:05Z
- src: testdata/deep-paths/
  dst: /bar
- src: ` + path + `
  dst: /foo/file.txt
`))
	dec.KnownFields(true)
	err := dec.Decode(&config)
	require.NoError(t, err)
	require.Len(t, config.Contents, 3)
	parsedContents, err := files.PrepareForPackager(
		config.Contents,
		0o133,
		"",
		false,
		mtime,
	)
	require.NoError(t, err)
	for _, c := range parsedContents {
		switch c.Source {
		case "testdata/globtest/nested/b.txt":
			require.Equal(t, "/bla/nested/b.txt", c.Destination)
			require.Equal(t, "-rw-r--r--", c.Mode().String())
		case "testdata/globtest/multi-nested/subdir/c.txt":
			require.Equal(t, "/bla/multi-nested/subdir/c.txt", c.Destination)
			require.Equal(t, "-rw-r--r--", c.Mode().String())
		case "testdata/deep-paths/nested1/nested2/a.txt":
			require.Equal(t, "/bar/nested1/nested2/a.txt", c.Destination)
			require.Equal(t, "-rw-r--r--", c.Mode().String())
		case path:
			require.Equal(t, "/foo/file.txt", c.Destination)
			require.Equal(t, "-rw-r--r--", c.Mode().String())
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
	parsedContents, err := files.PrepareForPackager(
		config.Contents,
		0,
		"",
		true,
		mtime,
	)
	require.NoError(t, err)
	present := false

	for _, f := range parsedContents {
		switch f.Source {
		case "testdata/deep-paths/nested1/nested2/a.txt":
			present = true
			require.Equal(t, "/bla/nested1/nested2/a.txt", f.Destination)
		case "":
			continue
		default:
			t.Errorf("unknown source %s for content %#v", f.Source, f)
		}
	}

	require.True(t, present)
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

	config.Contents, err = files.PrepareForPackager(
		config.Contents,
		0,
		"",
		true,
		mtime,
	)
	require.NoError(t, err)
	require.Len(t, config.Contents, 1)

	fi, err := os.Stat("files_test.go")
	require.NoError(t, err)

	f := config.Contents[0]
	require.Equal(t, "files_test.go", f.Source)
	require.Equal(t, "/b", f.Destination)
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

	config.Contents, err = files.PrepareForPackager(
		config.Contents,
		0,
		"rpm",
		true,
		mtime,
	)
	require.NoError(t, err)
	require.Len(t, config.Contents, 1)

	ct, err := time.Parse(time.RFC3339, "2008-01-02T15:04:05Z")
	require.NoError(t, err)

	f := config.Contents[0]
	require.Equal(t, "files_test.go", f.Source)
	require.Equal(t, "/b", f.Destination)
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

	config.Contents, err = files.PrepareForPackager(
		config.Contents,
		0,
		"",
		true,
		mtime,
	)
	require.NoError(t, err)
	config.Contents = withoutFileInfo(config.Contents)

	expected := files.Contents{
		{
			Source:      "",
			Destination: "/bla/",
			Type:        files.TypeImplicitDir,
		},
		{
			Source:      "testdata/symlinks/subdir/existent",
			Destination: "/bla/existent",
			Type:        files.TypeFile,
		},
		{
			Source:      "non-existent",
			Destination: "/bla/link-1",
			Type:        files.TypeSymlink,
		},
		{
			Source:      "existent",
			Destination: "/bla/link-2",
			Type:        files.TypeSymlink,
		},
		{
			Source:      "existent",
			Destination: "/bla/link-3",
			Type:        files.TypeSymlink,
		},
		{
			Source:      "broken",
			Destination: "/link-1",
			Type:        files.TypeSymlink,
		},
		{
			Source:      "bla",
			Destination: "/link-2",
			Type:        files.TypeSymlink,
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
			_, err := files.PrepareForPackager(
				config.Contents,
				0,
				"",
				false,
				mtime,
			)
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

		_, err := files.PrepareForPackager(
			configuredFiles,
			0,
			"",
			true,
			mtime,
		)
		require.ErrorIs(t, err, files.ErrContentCollision)
	})

	t.Run("no collision due to different per-file packagers", func(t *testing.T) {
		configuredFiles := []*files.Content{
			{Source: "../testdata/whatever.conf", Destination: "/samedestination", Packager: "foo"},
			{Source: "../testdata/whatever2.conf", Destination: "/samedestination", Packager: "bar"},
		}

		_, err := files.PrepareForPackager(
			configuredFiles,
			0,
			"foo",
			true,
			mtime,
		)
		require.NoError(t, err)
	})

	t.Run("collision between file for all packagers and file for specific packager", func(t *testing.T) {
		configuredFiles := []*files.Content{
			{Source: "../testdata/whatever.conf", Destination: "/samedestination", Packager: "foo"},
			{Source: "../testdata/whatever2.conf", Destination: "/samedestination", Packager: ""},
		}

		_, err := files.PrepareForPackager(
			configuredFiles,
			0,
			"foo",
			true,
			mtime,
		)
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
			result, err := files.PrepareForPackager(
				files.Contents{&content},
				0,
				"",
				disableGlobbing,
				mtime,
			)
			if err != nil {
				t.Fatalf("expand content globs: %v", err)
			}

			result = withoutImplicitDirs(result)

			if len(result) != 1 {
				t.Fatalf("unexpected result length: %d, expected one", len(result))
			}

			actualContent := result[0]

			// we expect the result content to be identical to the input content
			if actualContent.Source != content.Source {
				t.Fatalf("unexpected content source: %q, expected %q", actualContent.Source, content.Source)
			}

			if strings.TrimLeft(actualContent.Destination, "./") != strings.TrimLeft(content.Destination, "/") {
				t.Fatalf("unexpected content destination: %q, expected %q",
					strings.TrimLeft(actualContent.Destination, "./"), strings.TrimLeft(content.Destination, "/"))
			}
		})
	}
}

func withoutImplicitDirs(contents files.Contents) files.Contents {
	filtered := make(files.Contents, 0, len(contents))

	for _, c := range contents {
		if c.Type != files.TypeImplicitDir {
			filtered = append(filtered, c)
		}
	}

	return filtered
}

func TestGlobbingWhenFilesHaveBrackets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("doesn't work on windows")
	}
	result, err := files.PrepareForPackager(
		files.Contents{
			{
				Source:      "./testdata/\\{test\\}/",
				Destination: ".",
			},
		},
		0,
		"",
		false,
		mtime,
	)
	if err != nil {
		t.Fatalf("expand content globs: %v", err)
	}

	expected := files.Contents{
		{
			Source:      "testdata/{test}/[f]oo",
			Destination: "/[f]oo",
		},
		{
			Source:      "testdata/{test}/bar",
			Destination: "/bar",
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
			t.Fatalf("unexpected content destination: %q, expected %q",
				ex.Destination, r.Destination)
		}
	}
}

func TestGlobbingFilesWithDifferentSizesWithFileInfo(t *testing.T) {
	result, err := files.PrepareForPackager(
		files.Contents{
			{
				Source:      "./testdata/globtest/different-sizes/**/*",
				Destination: ".",
				FileInfo: &files.ContentFileInfo{
					Mode: 0o777,
				},
			},
		},
		0,
		"",
		false,
		mtime,
	)
	if err != nil {
		t.Fatalf("expand content globs: %v", err)
	}

	result = withoutImplicitDirs(result)

	if len(result) != 2 {
		t.Fatalf("unexpected result length: %d, expected 2", len(result))
	}

	if result[0].FileInfo.Size == result[1].FileInfo.Size {
		t.Fatal("test FileInfos have the same size, expected different")
	}
}

func TestDestEndsWithSlash(t *testing.T) {
	result, err := files.PrepareForPackager(
		files.Contents{
			{
				Source:      "./testdata/globtest/a.txt",
				Destination: "./foo/",
			},
		},
		0,
		"",
		false,
		mtime,
	)
	result = withoutImplicitDirs(result)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "/foo/a.txt", result[0].Destination)
}

func TestInvalidFileType(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: testdata/globtest/**/*
  dst: /bla
  type: filr
`))
	dec.KnownFields(true)
	require.NoError(t, dec.Decode(&config))
	_, err := files.PrepareForPackager(
		config.Contents,
		0,
		"",
		false,
		mtime,
	)
	require.EqualError(t, err, "invalid content type: filr")
}

func TestValidFileTypes(t *testing.T) {
	var config testStruct
	dec := yaml.NewDecoder(strings.NewReader(`---
contents:
- src: testdata/globtest/a.txt
  dst: /f1.txt
- src: testdata/globtest/a.txt
  dst: /f2.txt
  type: file
- src: testdata/globtest/a.txt
  dst: /f3.txt
  type: config
- src: testdata/globtest/a.txt
  dst: /f4.txt
  type: config|noreplace
- src: testdata/globtest/a.txt
  dst: /f5.txt
  type: symlink
- src: testdata/globtest/a.txt
  dst: /f6.txt
  type: dir
- src: testdata/globtest/a.txt
  dst: /f7.txt
  type: ghost
`))
	dec.KnownFields(true)
	require.NoError(t, dec.Decode(&config))
	_, err := files.PrepareForPackager(
		config.Contents,
		0,
		"",
		false,
		mtime,
	)
	require.NoError(t, err)
}

func TestImplicitDirectories(t *testing.T) {
	results, err := files.PrepareForPackager(
		files.Contents{
			{
				Source:      "./testdata/globtest/a.txt",
				Destination: "./foo/bar/baz",
			},
		},
		0,
		"",
		false,
		mtime,
	)
	require.NoError(t, err)

	expected := files.Contents{
		{
			Source:      "",
			Destination: "/foo/",
			Type:        files.TypeImplicitDir,
		},
		{
			Source:      "",
			Destination: "/foo/bar/",
			Type:        files.TypeImplicitDir,
		},
		{
			Source:      "testdata/globtest/a.txt",
			Destination: "/foo/bar/baz",
			Type:        files.TypeFile,
		},
	}

	require.Equal(t, expected, withoutFileInfo(results))
}

func TestRelevantFiles(t *testing.T) {
	contents := files.Contents{
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/1allpackagers",
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/2onlyrpm",
			Packager:    "rpm",
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/3onlydeb",
			Packager:    "deb",
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/4debchangelog",
			Type:        files.TypeDebChangelog,
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/5ghost",
			Type:        files.TypeRPMGhost,
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/6doc",
			Type:        files.TypeRPMDoc,
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/7licence",
			Type:        files.TypeRPMLicence,
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/8license",
			Type:        files.TypeRPMLicense,
		},
		{
			Source:      "./testdata/globtest/a.txt",
			Destination: "/9readme",
			Type:        files.TypeRPMReadme,
		},
	}

	t.Run("deb", func(t *testing.T) {
		results, err := files.PrepareForPackager(
			contents,
			0,
			"deb",
			false,
			mtime,
		)
		require.NoError(t, err)
		require.Equal(t, files.Contents{
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/1allpackagers",
				Type:        files.TypeFile,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/3onlydeb",
				Packager:    "deb",
				Type:        files.TypeFile,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/4debchangelog",
				Type:        files.TypeDebChangelog,
			},
		}, withoutFileInfo(results))
	})

	t.Run("rpm", func(t *testing.T) {
		results, err := files.PrepareForPackager(
			contents,
			0,
			"rpm",
			false,
			mtime,
		)
		require.NoError(t, err)
		require.Equal(t, files.Contents{
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/1allpackagers",
				Type:        files.TypeFile,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/2onlyrpm",
				Packager:    "rpm",
				Type:        files.TypeFile,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/5ghost",
				Type:        files.TypeRPMGhost,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/6doc",
				Type:        files.TypeRPMDoc,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/7licence",
				Type:        files.TypeRPMLicence,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/8license",
				Type:        files.TypeRPMLicense,
			},
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/9readme",
				Type:        files.TypeRPMReadme,
			},
		}, withoutFileInfo(results))
	})

	t.Run("apk", func(t *testing.T) {
		results, err := files.PrepareForPackager(
			contents,
			0,
			"apk",
			false,
			mtime,
		)
		require.NoError(t, err)
		require.Equal(t, files.Contents{
			{
				Source:      "testdata/globtest/a.txt",
				Destination: "/1allpackagers",
				Type:        files.TypeFile,
			},
		}, withoutFileInfo(results))
	})
}

func TestTreeOwner(t *testing.T) {
	results, err := files.PrepareForPackager(
		files.Contents{
			{
				Source:      filepath.Join("testdata", "tree"),
				Destination: "/usr/share/foo",
				Type:        files.TypeTree,
				FileInfo: &files.ContentFileInfo{
					Owner: "goreleaser",
					Group: "goreleaser",
					MTime: mtime,
				},
			},
		},
		0,
		"",
		false,
		mtime,
	)
	require.NoError(t, err)

	for _, f := range results {
		if f.Destination == "/usr/" || f.Destination == "/usr/share/" {
			require.Equal(t, "root", f.FileInfo.Owner, f.Destination)
			require.Equal(t, "root", f.FileInfo.Group, f.Destination)
			continue
		}
		require.Equal(t, "goreleaser", f.FileInfo.Owner, f.Destination)
		require.Equal(t, "goreleaser", f.FileInfo.Group, f.Destination)
	}

	require.Equal(t, files.Contents{
		{
			Source:      "",
			Destination: "/usr/",
			Type:        files.TypeImplicitDir,
		},
		{
			Source:      "",
			Destination: "/usr/share/",
			Type:        files.TypeImplicitDir,
		},
		{
			Source:      "",
			Destination: "/usr/share/foo/",
			Type:        files.TypeDir,
		},
		{
			Source:      "",
			Destination: "/usr/share/foo/files/",
			Type:        files.TypeDir,
		},
		{
			Source:      filepath.Join("testdata", "tree", "files", "a"),
			Destination: "/usr/share/foo/files/a",
			Type:        files.TypeFile,
		},
		{
			Source:      "",
			Destination: "/usr/share/foo/files/b/",
			Type:        files.TypeDir,
		},
		{
			Source:      filepath.Join("testdata", "tree", "files", "b", "c"),
			Destination: "/usr/share/foo/files/b/c",
			Type:        files.TypeFile,
		},
		{
			Source:      "",
			Destination: "/usr/share/foo/symlinks/",
			Type:        files.TypeDir,
		},
		{
			Source:      "/etc/foo",
			Destination: "/usr/share/foo/symlinks/link1",
			Type:        files.TypeSymlink,
		},
		{
			Source:      "../files/a",
			Destination: "/usr/share/foo/symlinks/link2",
			Type:        files.TypeSymlink,
		},
	}, withoutFileInfo(results))
}

func TestTree(t *testing.T) {
	results, err := files.PrepareForPackager(
		files.Contents{
			{
				Source:      filepath.Join("testdata", "tree"),
				Destination: "/base",
				Type:        files.TypeTree,
			},
		},
		0,
		"",
		false,
		mtime,
	)
	require.NoError(t, err)

	require.Equal(t, files.Contents{
		{
			Source:      "",
			Destination: "/base/",
			Type:        files.TypeDir,
		},
		{
			Source:      "",
			Destination: "/base/files/",
			Type:        files.TypeDir,
		},
		{
			Source:      filepath.Join("testdata", "tree", "files", "a"),
			Destination: "/base/files/a",
			Type:        files.TypeFile,
		},
		{
			Source:      "",
			Destination: "/base/files/b/",
			Type:        files.TypeDir,
		},
		{
			Source:      filepath.Join("testdata", "tree", "files", "b", "c"),
			Destination: "/base/files/b/c",
			Type:        files.TypeFile,
		},
		{
			Source:      "",
			Destination: "/base/symlinks/",
			Type:        files.TypeDir,
		},
		{
			Source:      "/etc/foo",
			Destination: "/base/symlinks/link1",
			Type:        files.TypeSymlink,
		},
		{
			Source:      "../files/a",
			Destination: "/base/symlinks/link2",
			Type:        files.TypeSymlink,
		},
	}, withoutFileInfo(results))
}

func withoutFileInfo(contents files.Contents) files.Contents {
	filtered := make(files.Contents, 0, len(contents))

	for _, c := range contents {
		cc := *c
		cc.FileInfo = nil
		filtered = append(filtered, &cc)
	}

	return filtered
}

func TestAsRelativePath(t *testing.T) {
	sep := fmt.Sprintf("%c", filepath.Separator)
	testCases := map[string]string{
		"/etc/foo/":         "etc/foo/",
		"./etc/foo":         "etc/foo",
		"./././foo/../bar/": "bar/",
		sep:                 "",
		sep + sep:           "",
		sep + strings.Join([]string{"foo", "bar", "zazz"}, sep): "foo/bar/zazz",
	}

	for input, expected := range testCases {
		assert.Equal(t, expected, files.AsRelativePath(input))
	}
}

func TestAsExplicitRelativePath(t *testing.T) {
	sep := fmt.Sprintf("%c", filepath.Separator)
	testCases := map[string]string{
		"/etc/foo/":         "./etc/foo/",
		"./etc/foo":         "./etc/foo",
		"./././foo/../bar/": "./bar/",
		sep:                 "./",
		sep:                 "./",
		sep + strings.Join([]string{"foo", "bar", "zazz"}, sep): "./foo/bar/zazz",
	}

	for input, expected := range testCases {
		assert.Equal(t, expected, files.AsExplicitRelativePath(input))
	}
}
