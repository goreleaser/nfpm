// Package glob provides file globbing for use in nfpm.Packager implementations
package glob

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goreleaser/fileglob"
)

// longestCommonPrefix returns the longest prefix of all strings the argument
// slice. If the slice is empty the empty string is returned.
func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	lcp := strs[0]
	for _, str := range strs {
		lcp = strlcp(lcp, str)
	}
	return lcp
}

func strlcp(a, b string) string {
	var min int
	if len(a) > len(b) {
		min = len(b)
	} else {
		min = len(a)
	}
	for i := 0; i < min; i++ {
		if a[i] != b[i] {
			return a[0:i]
		}
	}
	return a[0:min]
}

// ErrGlobNoMatch happens when no files matched the given glob.
type ErrGlobNoMatch struct {
	glob string
}

func (e ErrGlobNoMatch) Error() string {
	return fmt.Sprintf("glob failed: %s: no matching files", e.glob)
}

// Glob returns a map with source file path as keys and destination as values.
// First the longest common prefix (lcp) of all globbed files is found. The destination
// for each globbed file is then dst joined with src with the lcp trimmed off.
func Glob(pattern, dst string, ignoreMatchers bool) (map[string]string, error) {
	options := []fileglob.OptFunc{fileglob.MatchDirectoryIncludesContents}
	if ignoreMatchers {
		options = append(options, fileglob.QuoteMeta)
	}

	if strings.HasPrefix(pattern, "../") {
		p, err := filepath.Abs(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve pattern: %s: %w", pattern, err)
		}
		pattern = filepath.ToSlash(p)
	}

	matches, err := fileglob.Glob(pattern, append(options, fileglob.MaybeRootFS)...)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		return nil, fmt.Errorf("glob failed: %s: %w", pattern, err)
	}

	if len(matches) == 0 {
		return nil, ErrGlobNoMatch{pattern}
	}

	files := make(map[string]string)
	prefix := pattern
	// the prefix may not be a complete path or may use glob patterns, in that case use the parent directory
	if _, err := os.Stat(prefix); os.IsNotExist(err) || (fileglob.ContainsMatchers(pattern) && !ignoreMatchers) {
		prefix = filepath.Dir(longestCommonPrefix(matches))
	}

	for _, src := range matches {
		// only include files
		if f, err := os.Stat(src); err == nil && f.Mode().IsDir() {
			continue
		}

		relpath, err := filepath.Rel(prefix, src)
		if err != nil {
			// since prefix is a prefix of src a relative path should always be found
			return nil, err
		}

		globdst := filepath.ToSlash(filepath.Join(dst, relpath))
		files[src] = globdst
	}

	return files, nil
}
