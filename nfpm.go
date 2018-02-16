// Package nfpm provides ways to package programs in some linux packaging
// formats.
package nfpm

import (
	"fmt"
	"io"
	"sync"
)

var (
	packagers = map[string]Packager{}
	lock      sync.Mutex
)

// Register a new packager for the given format
func Register(format string, p Packager) {
	lock.Lock()
	packagers[format] = p
	lock.Unlock()
}

// Get a packager for the given format
func Get(format string) (Packager, error) {
	p, ok := packagers[format]
	if !ok {
		return nil, fmt.Errorf("no packager registered for the format %s", format)
	}
	return p, nil
}

// Packager represents any packager implementation
type Packager interface {
	Package(info Info, w io.Writer) error
}

// Info contains information about the package
type Info struct {
	Name        string            `yaml:"name"`
	Arch        string            `yaml:"arch"`
	Platform    string            `yaml:"platform"`
	Version     string            `yaml:"version"`
	Section     string            `yaml:"section"`
	Priority    string            `yaml:"priority"`
	Replaces    []string          `yaml:"replaces"`
	Provides    []string          `yaml:"provides"`
	Depends     []string          `yaml:"depends"`
	Conflicts   []string          `yaml:"conflicts"`
	Maintainer  string            `yaml:"maintainer"`
	Description string            `yaml:"description"`
	Vendor      string            `yaml:"vendor"`
	Homepage    string            `yaml:"homepage"`
	License     string            `yaml:"license"`
	Bindir      string            `yaml:"bindir"`
	Files       map[string]string `yaml:"files"`
	ConfigFiles map[string]string `yaml:"config_files"`
}

// WithDefaults set some sane defaults into the given Info
func WithDefaults(info Info) Info {
	if info.Bindir == "" {
		info.Bindir = "/usr/local/bin"
	}
	if info.Platform == "" {
		info.Platform = "linux"
	}
	return info
}
