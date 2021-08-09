package files

import (
	"os"
	"time"
)

type EmptyFolder struct {
	Path  string      `yaml:"path,omitempty"`
	Owner string      `yaml:"owner,omitempty"`
	Group string      `yaml:"group"`
	Mode  os.FileMode `yaml:"mode,omitempty"`
	MTime time.Time   `yaml:"mtime,omitempty"`
}

type EmptyFolders []*EmptyFolder

func (f EmptyFolders) WithFolderDefaults() EmptyFolders {
	for index, folder := range f {
		if folder.Owner == "" {
			(f)[index].Owner = "root"
		}
		if folder.Group == "" {
			(f)[index].Group = "root"
		}
		if folder.MTime.IsZero() {
			(f)[index].MTime = time.Now().UTC()
		}
	}
	return f
}
