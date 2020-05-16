// Package tarhelper contains helpers for conveniently producing tar files.
package tarhelper

import (
	"archive/tar"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/vbatts/go-mtree"
)

func cleanPath(p string) string {
	return strings.Trim(path.Clean(filepath.ToSlash(p)), "/")
}

// dirChain computes all ancestors of the given path, shortest first. The
// argument must be a cleaned, slash-delimited path not starting with "/" or
// equal to "." or "".
func dirChain(dst string) []string {
	words := strings.Split(dst, "/")
	result := []string{words[0]}
	for i := 1; i < len(words); i++ {
		result = append(result, result[i-1]+"/"+words[i])
	}
	return result
}

// File is a wrapper around a tar.Writer that provides convenience methods for
// adding files and empty directories and automatically adds parent directories
// for all files. It also supports producing an mtree representation of its
// contents.
type File struct {
	writer        *tar.Writer
	mtreeKeywords []mtree.Keyword
	createdDirs   map[string]bool
	entries       []mtree.Entry
}

func New(writer *tar.Writer, keywords []string) *File {
	return &File{
		writer:        writer,
		mtreeKeywords: mtree.ToKeywords(keywords),
		createdDirs:   make(map[string]bool),
	}
}

// MkdirAll creates an empty directory at the given path, along with all of its
// parents.
func (t *File) MkdirAll(dst string) error {
	dst = cleanPath(dst)
	if dst == "" || dst == "." {
		return nil
	}
	for _, dir := range dirChain(dst) {
		if t.createdDirs[dir] {
			continue
		}
		t.createdDirs[dir] = true

		if err := t.writer.WriteHeader(&tar.Header{
			Name:     dir + "/",
			Mode:     0o755,
			Typeflag: tar.TypeDir,
			ModTime:  time.Unix(0, 0),
		}); err != nil {
			return errors.Wrap(err, "failed to create directory")
		}
		t.entries = append(t.entries, mtree.Entry{
			Name:     dir + "/",
			Keywords: []mtree.KeyVal{"mode=0755", "type=dir", "time=0"},
		})
	}
	return nil
}

// AddFile adds a file to the tar file, automatically creating parent
// directories.
func (t *File) AddFile(dst string, stat os.FileInfo, f io.ReadSeeker) error {
	dst = cleanPath(dst)

	if err := t.MkdirAll(filepath.Dir(dst)); err != nil {
		return err
	}

	err := t.writer.WriteHeader(&tar.Header{
		Name:    dst,
		Mode:    int64(stat.Mode()),
		Size:    stat.Size(),
		ModTime: stat.ModTime(),
	})
	if err != nil {
		return err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(t.writer, f)
	if err != nil {
		return err
	}

	e := mtree.Entry{Name: dst}
	for _, keyword := range t.mtreeKeywords {
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		kvs, err := mtree.KeywordFuncs[keyword](dst, stat, f)
		if err != nil {
			return err
		}
		for _, kv := range kvs {
			if kv != "" {
				e.Keywords = append(e.Keywords, kv)
			}
		}
	}
	t.entries = append(t.entries, e)

	return nil
}

// MtreeHierarchy returns an mtree directory hierarchy representing the files
// and directories that have been added.
func (t *File) MtreeHierarchy() *mtree.DirectoryHierarchy {
	return &mtree.DirectoryHierarchy{Entries: t.entries}
}
