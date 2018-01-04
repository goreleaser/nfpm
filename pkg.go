package pkg

import "io"

type Packager interface {
	io.Closer
	Add(src, dst string) error
}

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
