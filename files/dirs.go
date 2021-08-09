package files

import (
	"os"
	"time"
)

type EmptyFolder struct {
	Path  string      `yaml:"path,omitempty" jsonschema:"title=path"`
	Owner string      `yaml:"owner,omitempty" jsonschema:"title=owner"`
	Group string      `yaml:"group" jsonschema:"title=group"`
	Mode  os.FileMode `yaml:"mode,omitempty" jsonschema:"title=mode"`
	MTime time.Time   `yaml:"mtime,omitempty" jsonschema:"title=modified time"`
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
