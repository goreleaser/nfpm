// Package arch implements nfpm.Packager providing Arch Linux package bindings.
package arch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"github.com/goreleaser/nfpm"
	"github.com/goreleaser/nfpm/internal/tarhelper"
)

// nolint: gochecknoinits
func init() {
	nfpm.Register("gz", Default)
}

// Default Arch Linux packager
// nolint: gochecknoglobals
var Default = &Arch{}

// Arch is an Arch Linux packager implementation.
type Arch struct{}

// nolint: gochecknoglobals
var archToArch = map[string]string{
	"386":   "i686",
	"amd64": "x86_64",
	"arm5":  "armv5h",
	"arm6":  "armv6h",
	"arm64": "aarch64",
	"arm7":  "armv7h",
}

// Package writes a new Arch Linux package to the given writer using the given
// info.
func (a *Arch) Package(info *nfpm.Info, arch io.Writer) (err error) {
	if arch, ok := archToArch[info.Arch]; ok {
		info.Arch = arch
	}

	// set up output
	gz, _ := gzip.NewWriterLevel(arch, gzip.BestCompression)
	defer gz.Close()
	tarWriter := tar.NewWriter(gz)
	defer tarWriter.Close()
	tarFile := tarhelper.New(
		tarWriter,
		[]string{"mode", "time", "size", "type", "md5digest", "sha256digest"},
	)

	// add regular installed files
	instSize, err := a.addPackageFiles(info, tarFile)
	if err != nil {
		return errors.Wrap(err, "failed to add package files")
	}

	// add metadata files; .MTREE needs to be last, since it includes the other
	// metadata files
	pkgInfo, err := getPkgInfo(info, instSize)
	if err != nil {
		return errors.Wrap(err, "failed to create .PKGINFO")
	}
	f := fakeFile{".PKGINFO", pkgInfo}
	if err := tarFile.AddFile(f.path, f, f.Open()); err != nil {
		return errors.Wrap(err, "failed to add .PKGINFO")
	}

	if err := addInstallScript(info, tarFile); err != nil {
		return errors.Wrap(err, "failed to add .INSTALL")
	}
	if err := addMtree(tarFile); err != nil {
		return errors.Wrap(err, "failed to add .MTREE")
	}
	return nil
}

func (a *Arch) addPackageFiles(info *nfpm.Info, tarFile *tarhelper.File) (int64, error) {
	files, err := info.FilesToCopy()
	if err != nil {
		return 0, err
	}
	var instSize int64
	for _, file := range files {
		stat, err := os.Stat(file.Source)
		if err != nil {
			return 0, err
		}
		f, err := os.Open(file.Source)
		if err != nil {
			return 0, err
		}
		err = tarFile.AddFile(file.Destination, roundingFileInfo{stat}, f)
		f.Close()
		if err != nil {
			return 0, err
		}
		instSize += stat.Size()
	}

	for _, folder := range info.EmptyFolders {
		if err := tarFile.MkdirAll(folder); err != nil {
			return 0, err
		}
	}

	return instSize, nil
}

const pkgInfoTemplate = `pkgname = {{.Info.Name}}
pkgdesc = {{oneline .Info.Description}}
pkgver = {{ if .Info.Epoch}}{{ .Info.Epoch }}:{{ end }}{{.Info.Version}}-{{defaultRelease .Info.Release}}
size = {{.InstalledSize}}
url = {{.Info.Homepage}}
packager = {{.Info.Maintainer}}
{{- range .Info.Replaces}}
replaces = {{.}}
{{- end }}
{{- range .Info.Provides}}
provides = {{.}}
{{- end }}
{{- range .Info.Depends}}
depend = {{.}}
{{- end }}
{{- range .Info.Recommends}}
optdepend = {{.}}
{{- end }}
{{- range .Info.Suggests}}
optdepend = {{.}}
{{- end }}
{{- range .Info.Conflicts}}
conflict = {{.}}
{{- end }}
{{- range .Info.ArchLinux.Groups}}
group = {{.}}
{{- end }}
{{- range .Info.ConfigFiles}}
backup = {{.}}
{{- end}}
`

type controlData struct {
	Info          *nfpm.Info
	InstalledSize int64
}

func getPkgInfo(info *nfpm.Info, instSize int64) ([]byte, error) {
	buf := &bytes.Buffer{}
	tmpl := template.New("pkginfo")
	tmpl.Funcs(template.FuncMap{
		// the description field only supports single-line descriptions
		"oneline": func(str string) string {
			return strings.TrimRight(regexp.MustCompile(`\n\s*`).ReplaceAllString(str, " "), " \n")
		},
		// the full version must have a hyphen followed by the release; the release
		// can be empty (e.g., a version of "1.2.3-"), but that looks weird
		"defaultRelease": func(str string) string {
			if str == "" {
				return "1"
			}
			return str
		},
	})
	if err := template.Must(tmpl.Parse(pkgInfoTemplate)).Execute(buf, controlData{info, instSize}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func addInstallScript(info *nfpm.Info, tarFile *tarhelper.File) error {
	if info.ArchLinux.InstallScript == "" {
		return nil
	}
	stat, err := os.Stat(info.ArchLinux.InstallScript)
	if err != nil {
		return err
	}
	f, err := os.Open(info.ArchLinux.InstallScript)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = tarFile.AddFile(".INSTALL", roundingFileInfo{stat}, f); err != nil {
		return err
	}
	return nil
}

func addMtree(tarFile *tarhelper.File) error {
	buf := &bytes.Buffer{}
	gz, _ := gzip.NewWriterLevel(buf, gzip.BestCompression)
	_, _ = tarFile.MtreeHierarchy().WriteTo(gz)
	gz.Close()

	f := fakeFile{".MTREE", buf.Bytes()}
	if err := tarFile.AddFile(f.path, f, f.Open()); err != nil {
		return err
	}

	return nil
}
