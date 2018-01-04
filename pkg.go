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
	Filename    string
	Name        string
	Arch        string
	Version     string
	Section     string
	Priority    string
	Depends     []string
	Maintainer  string
	Description string
	Vendor      string
	Homepage    string
}
