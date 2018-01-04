// Package pkg provides ways to package programs in some linux packaging
// formats.
package pkg

import "io"

// Packager can package files in some format
type Packager interface {
	io.Closer
	Add(src, dst string) error
}

// Info contains information about the package
type Info struct {
	Name        string   `yaml:"name,omitempty"`
	Arch        string   `yaml:"arch,omitempty"`
	Version     string   `yaml:"version,omitempty"`
	Section     string   `yaml:"section,omitempty"`
	Priority    string   `yaml:"priority,omitempty"`
	Depends     []string `yaml:"depends,omitempty"`
	Maintainer  string   `yaml:"maintainer,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Vendor      string   `yaml:"vendor,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty"`
}
