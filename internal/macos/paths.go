package macos

import (
	"fmt"
	"path"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// PathKey returns the key two paths share when they name the same file on a
// default macOS volume, which ignores both letter case and Unicode
// normalization.
func PathKey(p string) string {
	return norm.NFD.String(cases.Fold().String(p))
}

// PathIndex detects distinct paths that macOS would store as one.
type PathIndex struct {
	spellings map[string]string
}

// NewPathIndex returns an empty PathIndex.
func NewPathIndex() *PathIndex {
	return &PathIndex{spellings: map[string]string{}}
}

// Add records a clean, absolute path and every one of its parent
// directories. It fails when a path was already recorded with a different
// spelling.
func (i *PathIndex) Add(p string) error {
	for current := p; current != "/" && current != "."; current = path.Dir(current) {
		key := PathKey(current)
		previous, seen := i.spellings[key]
		if !seen {
			i.spellings[key] = current
			continue
		}
		if previous != current {
			return fmt.Errorf(
				"package contents %q and %q differ only by letter case or Unicode normalization; "+
					"macOS stores them as the same path",
				previous, current)
		}
	}
	return nil
}
