package files

import (
	"sort"

	"github.com/goreleaser/fileglob"

	"github.com/goreleaser/nfpm/internal/glob"
)

// FileToCopy describes the source and destination
// of one file to copy into a package.
type FileToCopy struct {
	Source      string
	Destination string
}

// Expand gathers all of the real files to be copied into the package.
func Expand(filesSrcDstMap map[string]string, disableGlobbing bool) ([]FileToCopy, error) {
	var files []FileToCopy

	for srcGlob, dstRoot := range filesSrcDstMap {
		if disableGlobbing {
			srcGlob = fileglob.QuoteMeta(srcGlob)
		}

		globbed, err := glob.Glob(srcGlob, dstRoot)
		if err != nil {
			return nil, err // nolint:wrapcheck
		}
		for src, dst := range globbed {
			files = append(files, FileToCopy{src, dst})
		}
	}

	// sort the files for reproducibility and general cleanliness
	sort.Slice(files, func(i, j int) bool {
		a, b := files[i], files[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Destination < b.Destination
	})
	return files, nil
}
