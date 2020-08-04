package files

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goreleaser/nfpm"
	"github.com/goreleaser/nfpm/glob"
)

// FileToCopy describes the source and destination of one file to copy into a
// package and whether it is a config file.
type FileToCopy struct {
	Source      string
	Destination string
	Config      bool
}

// FilesToCopy lists all of the real files to be copied into the package.
func FilesToCopy(info *nfpm.Info) ([]FileToCopy, error) {
	var files []FileToCopy
	for i, filesMap := range []map[string]string{info.Files, info.ConfigFiles} {
		for srcglob, dstroot := range filesMap {
			globbed, err := glob.Glob(srcglob, dstroot)
			if err != nil {
				return nil, err
			}
			for src, dst := range globbed {
				// avoid including a partial file with the name of the target in the target
				// itself. when used as a lib, target may not be set. in that case, src will
				// always have the empty sufix, and all files will be ignored.
				// TODO: add tests cases for this
				if info.Target != "" && strings.HasSuffix(src, info.Target) {
					fmt.Printf("skipping %s because it has the suffix %s", src, info.Target)
					continue
				}

				files = append(files, FileToCopy{src, dst, i == 1})
			}
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
