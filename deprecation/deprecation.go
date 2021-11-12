// Package deprecation provides centralized deprecation notice messaging for nfpm.
package deprecation

import (
	"fmt"
	"io"
	"os"
)

var deprecationNoticer DeprecationNoticer

// DeprecationNoticer is the interface for deprecation noticer.
type DeprecationNoticer interface {
	Notice(s string)
}

// Notice will notice a deprecation with the previously set noticer.
func Notice(s string) {
	if deprecationNoticer == nil {
		Set(&defaultNoticer{
			w: os.Stderr,
		})
	}
	deprecationNoticer.Notice(s)
}

// Set the noticer implementation to use.
func Set(n DeprecationNoticer) {
	deprecationNoticer = n
}

type defaultNoticer struct {
	w io.Writer
}

func (d *defaultNoticer) Notice(s string) {
	fmt.Fprintln(d.w, "DEPRECATION WARNING: "+s)
}
