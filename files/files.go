package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/goreleaser/fileglob"
	"gopkg.in/yaml.v3"

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

func (c *Contents) UnmarshalYAML(value *yaml.Node) (err error) {
	// nolint:exhaustive
	// we do not care about `AliasNode`, `DocumentNode`
	switch value.Kind {
	case yaml.SequenceNode:
		type tmpContents Contents
		var tmp tmpContents
		if err = value.Decode(&tmp); err != nil {
			return err
		}
		*c = Contents(tmp)
	case yaml.ScalarNode:
		// TODO: implement issue-43 here and remove the fallthrough
		// nolint:gocritic
		// ignoring `emptyFallthrough: remove empty case containing only fallthrough to default case`
		fallthrough
	default:
		// nolint:goerr113
		// this is temporary so we do not need the a static error
		return fmt.Errorf("not implemented")
	}

	return nil
}

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

func (c *Content) WithFileInfoDefaults() {
	if c.FileInfo == nil {
		c.FileInfo = &ContentFileInfo{}
	}
	if c.FileInfo.Owner == "" {
		c.FileInfo.Owner = "root"
	}
	if c.FileInfo.Group == "" {
		c.FileInfo.Group = "root"
	}
	info, err := os.Stat(c.Source)
	if err == nil {
		c.FileInfo.MTime = info.ModTime()
		c.FileInfo.Mode = info.Mode()
		c.FileInfo.Size = info.Size()
	}
	if c.FileInfo.MTime.IsZero() {
		c.FileInfo.MTime = time.Now().UTC()
	}
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
func ExpandContentGlobs(filesSrcDstMap Contents, disableGlobbing bool) (files Contents, err error) {
	for _, f := range filesSrcDstMap {
		var globbed map[string]string
		if disableGlobbing {
			f.Source = fileglob.QuoteMeta(f.Source)
		}

		switch f.Type {
		case "ghost", "symlink":
			// Ghost and symlink files need to be in the list, but dont glob them because they do not really exist
			f.WithFileInfoDefaults()
			files = append(files, f)
			continue
		}

		globbed, err = glob.Glob(f.Source, f.Destination)
		if err != nil {
			return nil, err
		}
		files = appendGlobbedFiles(globbed, f, files)
	}

	// sort the files for reproducibility and general cleanliness
	sort.Sort(files)

	return files, nil
}

func appendGlobbedFiles(globbed map[string]string, origFile *Content, incFiles Contents) (files Contents) {
	files = append(files, incFiles...)
	for src, dst := range globbed {
		newFile := &Content{
			Destination: ToNixPath(dst),
			Source:      ToNixPath(src),
			Type:        origFile.Type,
			FileInfo:    origFile.FileInfo,
			Packager:    origFile.Packager,
		}
		newFile.WithFileInfoDefaults()
		files = append(files, newFile)
	}

	return files
}

// ToNixPath converts the given path to a nix-style path.
//
// Windows-style path separators are considered escape
// characters by some libraries, which can cause issues.
func ToNixPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
