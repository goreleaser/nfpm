package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/goreleaser/fileglob"
	"github.com/goreleaser/nfpm/v2/internal/glob"
)

// Content describes the source and destination
// of one file to copy into a package.
type Content struct {
	Source      string           `yaml:"src,omitempty"`
	Destination string           `yaml:"dst,omitempty"`
	Type        string           `yaml:"type,omitempty"`
	Packager    string           `yaml:"packager,omitempty"`
	FileInfo    *ContentFileInfo `yaml:"file_info,omitempty"`
}

type ContentFileInfo struct {
	Owner string      `yaml:"owner,omitempty"`
	Group string      `yaml:"group"`
	Mode  os.FileMode `yaml:"mode,omitempty"`
	MTime time.Time   `yaml:"mtime,omitempty"`
	Size  int64       `yaml:"-"`
}

// Contents list of Content to process.
type Contents []*Content

func (c Contents) Len() int {
	return len(c)
}

func (c Contents) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c Contents) Less(i, j int) bool {
	a, b := c[i], c[j]
	if a.Type != b.Type {
		return len(a.Type) < len(b.Type)
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.Destination < b.Destination
}

func (c *Content) WithFileInfoDefaults() *Content {
	cc := &Content{
		Source:      c.Source,
		Destination: c.Destination,
		Type:        c.Type,
		Packager:    c.Packager,
		FileInfo:    c.FileInfo,
	}
	if cc.FileInfo == nil {
		cc.FileInfo = &ContentFileInfo{}
	}
	if cc.FileInfo.Owner == "" {
		cc.FileInfo.Owner = "root"
	}
	if cc.FileInfo.Group == "" {
		cc.FileInfo.Group = "root"
	}
	info, err := os.Stat(cc.Source)
	if err == nil {
		if cc.FileInfo.MTime.IsZero() {
			cc.FileInfo.MTime = info.ModTime()
		}
		if cc.FileInfo.Mode == 0 {
			cc.FileInfo.Mode = info.Mode()
		}
		cc.FileInfo.Size = info.Size()
	}
	if cc.FileInfo.MTime.IsZero() {
		cc.FileInfo.MTime = time.Now().UTC()
	}
	return cc
}

// Name to part of the os.FileInfo interface
func (c *Content) Name() string {
	return c.Source
}

// Size to part of the os.FileInfo interface
func (c *Content) Size() int64 {
	return c.FileInfo.Size
}

// Mode to part of the os.FileInfo interface
func (c *Content) Mode() os.FileMode {
	return c.FileInfo.Mode
}

// ModTime to part of the os.FileInfo interface
func (c *Content) ModTime() time.Time {
	return c.FileInfo.MTime
}

// IsDir to part of the os.FileInfo interface
func (c *Content) IsDir() bool {
	return false
}

// Sys to part of the os.FileInfo interface
func (c *Content) Sys() interface{} {
	return nil
}

// ExpandContentGlobs gathers all of the real files to be copied into the package.
func ExpandContentGlobs(contents Contents, disableGlobbing bool) (files Contents, err error) {
	for _, f := range contents {
		var globbed map[string]string

		switch f.Type {
		case "ghost", "symlink":
			// Ghost and symlink files need to be in the list, but dont glob them because they do not really exist
			files, err = appendWithUniqueDestination(files, f.WithFileInfoDefaults())
			if err != nil {
				return nil, err
			}

			continue
		}

		options := []fileglob.OptFunc{fileglob.MatchDirectoryIncludesContents}
		if disableGlobbing {
			options = append(options, fileglob.QuoteMeta)
		}
		globbed, err = glob.Glob(f.Source, f.Destination, options...)
		if err != nil {
			return nil, err
		}

		files, err = appendGlobbedFiles(globbed, f, files)
		if err != nil {
			return nil, err
		}
	}

	// sort the files for reproducibility and general cleanliness
	sort.Sort(files)

	return files, nil
}

func appendGlobbedFiles(globbed map[string]string, origFile *Content, incFiles Contents) (files Contents, err error) {
	files = append(files, incFiles...)
	for src, dst := range globbed {
		newFile := &Content{
			Destination: ToNixPath(dst),
			Source:      ToNixPath(src),
			Type:        origFile.Type,
			FileInfo:    origFile.FileInfo,
			Packager:    origFile.Packager,
		}

		files, err = appendWithUniqueDestination(files, newFile.WithFileInfoDefaults())
		if err != nil {
			return nil, err
		}
	}

	return files, nil
}

var ErrContentCollision = fmt.Errorf("content collision")

func appendWithUniqueDestination(slice []*Content, elems ...*Content) ([]*Content, error) {
	alreadyPresent := map[string]*Content{}

	for _, presentElem := range slice {
		alreadyPresent[presentElem.Destination] = presentElem
	}

	for _, elem := range elems {
		present, ok := alreadyPresent[elem.Destination]
		if ok && (present.Packager == "" || elem.Packager == "" || present.Packager == elem.Packager) {
			return nil, fmt.Errorf(
				"cannot add %s because %s is already present at the same destination (%s): %w",
				elem.Source, present.Source, present.Destination, ErrContentCollision)
		}

		alreadyPresent[elem.Destination] = elem
	}

	return append(slice, elems...), nil
}

// ToNixPath converts the given path to a nix-style path.
//
// Windows-style path separators are considered escape
// characters by some libraries, which can cause issues.
func ToNixPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
