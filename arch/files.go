package arch

import (
	"bytes"
	"io"
	"os"
	"path"
	"time"
)

// fakeFile simulates a file. It implements os.FileInfo directly and provides a
// method allowing the file to be "opened" for reading.
type fakeFile struct {
	path    string
	content []byte
}

func (f fakeFile) Name() string        { return path.Base(f.path) }
func (f fakeFile) Size() int64         { return int64(len(f.content)) }
func (f fakeFile) Mode() os.FileMode   { return 0o644 }
func (f fakeFile) ModTime() time.Time  { return time.Unix(0, 0) }
func (f fakeFile) IsDir() bool         { return false }
func (f fakeFile) Sys() interface{}    { return nil }
func (f fakeFile) Open() io.ReadSeeker { return bytes.NewReader(f.content) }

// roundingFileInfo wraps an instance of os.FileInfo, differing only by rounding
// the modification time to the nearest second. It is used to avoid issues with
// different tools later on rounding non-integral times differently.
type roundingFileInfo struct {
	info os.FileInfo
}

func (f roundingFileInfo) Name() string       { return f.info.Name() }
func (f roundingFileInfo) Size() int64        { return f.info.Size() }
func (f roundingFileInfo) Mode() os.FileMode  { return f.info.Mode() }
func (f roundingFileInfo) ModTime() time.Time { return f.info.ModTime().Round(time.Second) }
func (f roundingFileInfo) IsDir() bool        { return f.info.IsDir() }
func (f roundingFileInfo) Sys() interface{}   { return f.info.Sys() }
