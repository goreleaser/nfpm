package ipk

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/goreleaser/nfpm/v2"
	"github.com/goreleaser/nfpm/v2/files"
	"github.com/stretchr/testify/require"
)

// extractDataTar returns the tar headers of the data.tar.gz member of an ipk,
// keyed by the entry name with the leading "./" removed.
func extractDataTar(t *testing.T, ipk []byte) map[string]*tar.Header {
	t.Helper()

	outer, err := gzip.NewReader(bytes.NewReader(ipk))
	require.NoError(t, err)
	outer.Multistream(true)

	outerTar := tar.NewReader(outer)
	for {
		header, err := outerTar.Next()
		if err == io.EOF {
			t.Fatal("data.tar.gz not found in ipk")
		}
		require.NoError(t, err)

		body, err := io.ReadAll(outerTar)
		require.NoError(t, err)

		if header.Name != "./data.tar.gz" {
			continue
		}

		zr, err := gzip.NewReader(bytes.NewReader(body))
		require.NoError(t, err)

		headers := map[string]*tar.Header{}
		dataTar := tar.NewReader(zr)
		for {
			inner, err := dataTar.Next()
			if err == io.EOF {
				return headers
			}
			require.NoError(t, err)
			headers[filepath.ToSlash(inner.Name)] = inner
		}
	}
}

// TestIPKTreeDirectoryModeIsNotBase256 asserts that a directory discovered by a
// `type: tree` entry is written with its permission bits only.
//
// files.PrepareForPackager stores the raw fs.FileMode reported by the tree walk
// in Content.FileInfo.Mode. For a directory that value carries fs.ModeDir
// (bit 31), so the raw 0x800001ED does not fit tar's 7-digit octal mode field.
// archive/tar then silently switches that single header to GNU format with
// base-256 (binary) numeric encoding, which ipk consumers cannot read. The apk
// packager had the same defect and fixed it in #1113 (see normalizeFileMode);
// ipk was left behind.
func TestIPKTreeDirectoryModeIsNotBase256(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(source, "sub"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(source, "sub", "f.txt"), []byte("hi"), 0o644))

	info := exampleInfo()
	info.Contents = files.Contents{{
		Source:      source,
		Destination: "/usr/share/doc/",
		Type:        files.TypeTree,
	}}

	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	headers := extractDataTar(t, buf.Bytes())

	treeDir, ok := headers["./usr/share/doc/sub/"]
	require.True(t, ok, "walked directory missing from data.tar.gz: %v", headers)

	// Derive the expectation from what the walk actually saw instead of
	// hardcoding 0o755: MkdirAll/WriteFile are masked by the process umask, so
	// a restrictive umask (0o077) would otherwise fail this on mode, not on the
	// base-256 encoding this test is about. This mirrors the apk packager.
	subInfo, err := os.Stat(filepath.Join(source, "sub"))
	require.NoError(t, err)
	fileInfo, err := os.Stat(filepath.Join(source, "sub", "f.txt"))
	require.NoError(t, err)

	// The mode must fit the 7-digit octal field, which means the fs.ModeDir bit
	// has to be gone. Anything >= 1<<23 forces base-256 encoding.
	require.EqualValues(t, subInfo.Mode().Perm()&^info.Umask, treeDir.Mode,
		"walked tree directory must carry permission bits only")
	require.Less(t, treeDir.Mode, int64(1<<23),
		"mode overflows the tar octal field and gets base-256 encoded")
	require.NotEqual(t, byte(0x80), rawModeField(t, treeDir)[0],
		"mode field is base-256 encoded, not octal ASCII")

	// Sanity check: a regular file in the same tree is unaffected.
	require.EqualValues(t, fileInfo.Mode().Perm()&^info.Umask,
		headers["./usr/share/doc/sub/f.txt"].Mode)
}

// rawModeField re-encodes a header the way archive/tar would and returns the
// first byte of the mode field, so the test can assert on the actual on-disk
// encoding rather than the parsed value.
func rawModeField(t *testing.T, h *tar.Header) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(h))
	require.NoError(t, tw.Close())
	return buf.Bytes()[100:108]
}

// TestNormalizeIPKFileMode covers the mode conversion itself, mirroring the
// table test the apk packager got in #1113.
func TestNormalizeIPKFileMode(t *testing.T) {
	for name, tt := range map[string]struct {
		mode fs.FileMode
		want int64
	}{
		"permissions":          {mode: 0o755, want: 0o755},
		"directory type":       {mode: fs.ModeDir | 0o755, want: 0o755},
		"nested directory":     {mode: fs.ModeDir | 0o700, want: 0o700},
		"Go setuid":            {mode: fs.ModeSetuid | 0o755, want: 0o4755},
		"Go setgid":            {mode: fs.ModeSetgid | 0o755, want: 0o2755},
		"Go sticky":            {mode: fs.ModeSticky | 0o755, want: 0o1755},
		"octal special bits":   {mode: 0o7755, want: 0o7755},
		"unrelated type flags": {mode: fs.ModeSymlink | 0o777, want: 0o777},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, normalizeIPKFileMode(tt.mode))
		})
	}
}

// TestIPKTreePreservesSpecialModeBits makes sure the fix does not regress the
// opposite direction: setuid/setgid/sticky must survive the conversion.
//
// The mode is configured explicitly rather than chmod'ed on disk, because the
// mode is taken from the walk and some filesystems (btrfs with nosuid, for
// example) silently drop the setuid bit, which would make this test
// filesystem-dependent.
func TestIPKTreePreservesSpecialModeBits(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "suid"), []byte("x"), 0o644))

	info := exampleInfo()
	info.Contents = files.Contents{{
		Source:      source,
		Destination: "/usr/bin/",
		Type:        files.TypeTree,
		FileInfo:    &files.ContentFileInfo{Mode: fs.ModeSetuid | fs.ModeSetgid | 0o755},
	}}

	var buf bytes.Buffer
	require.NoError(t, Default.Package(info, &buf))

	headers := extractDataTar(t, buf.Bytes())
	require.EqualValues(t, 0o6755, headers["./usr/bin/suid"].Mode)
	require.EqualValues(t, 0o6755, normalizeIPKFileMode(fs.ModeSetuid|fs.ModeSetgid|0o755))
}

var _ = nfpm.Info{}
