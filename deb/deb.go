// Package deb implements nfpm.Packager providing .deb bindings.
package deb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/blakesmith/ar"
	"github.com/goreleaser/nfpm"
	"github.com/pkg/errors"
)

func init() {
	nfpm.Register("deb", Default)
}

// Default deb packager
var Default = &Deb{}

// Deb is a deb packager implementation
type Deb struct{}

// Package writes a new deb package to the given writer using the given info
func (*Deb) Package(info nfpm.Info, deb io.Writer) (err error) {
	var now = time.Now()
	dataTarGz, md5sums, instSize, err := createDataTarGz(now, info)
	if err != nil {
		return err
	}
	controlTarGz, err := createControl(now, instSize, md5sums, info)
	if err != nil {
		return err
	}
	var w = ar.NewWriter(deb)
	if err := w.WriteGlobalHeader(); err != nil {
		return errors.Wrap(err, "cannot write ar header to deb file")
	}
	if err := addArFile(now, w, "debian-binary", []byte("2.0\n")); err != nil {
		return errors.Wrap(err, "cannot pack debian-binary")
	}
	if err := addArFile(now, w, "control.tar.gz", controlTarGz); err != nil {
		return errors.Wrap(err, "cannot add control.tar.gz to deb")
	}
	if err := addArFile(now, w, "data.tar.gz", dataTarGz); err != nil {
		return errors.Wrap(err, "cannot add data.tar.gz to deb")
	}
	return nil
}

func addArFile(now time.Time, w *ar.Writer, name string, body []byte) error {
	var header = ar.Header{
		Name:    name,
		Size:    int64(len(body)),
		Mode:    0644,
		ModTime: now,
	}
	if err := w.WriteHeader(&header); err != nil {
		return errors.Wrap(err, "cannot write file header")
	}
	_, err := w.Write(body)
	return err
}

func createDataTarGz(now time.Time, info nfpm.Info) (dataTarGz, md5sums []byte, instSize int64, err error) {
	var buf bytes.Buffer
	var compress = gzip.NewWriter(&buf)
	var out = tar.NewWriter(compress)
	defer out.Close()
	defer compress.Close()

	var md5buf bytes.Buffer
	var md5tmp = make([]byte, 0, md5.Size)

	for _, files := range []map[string]string{info.Files, info.ConfigFiles} {
		for src, dst := range files {
			file, err := os.Open(src)
			if err != nil {
				return nil, nil, 0, errors.Wrapf(err, "could not open %s", src)
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil || info.IsDir() {
				continue
			}
			instSize += info.Size()
			var header = tar.Header{
				Name:    dst,
				Size:    info.Size(),
				Mode:    int64(info.Mode()),
				ModTime: now,
			}
			if err := out.WriteHeader(&header); err != nil {
				return nil, nil, 0, errors.Wrapf(err, "cannot write header of %s to data.tar.gz", header)
			}
			if _, err := io.Copy(out, file); err != nil {
				return nil, nil, 0, errors.Wrapf(err, "cannot write %s to data.tar.gz", header)
			}

			var digest = md5.New()
			if _, err := io.Copy(out, io.TeeReader(file, digest)); err != nil {
				return nil, nil, 0, errors.Wrap(err, "failed to copy")
			}
			fmt.Fprintf(&md5buf, "%x  %s\n", digest.Sum(md5tmp), header.Name[2:])
		}
	}

	if err := out.Close(); err != nil {
		return nil, nil, 0, errors.Wrap(err, "closing data.tar.gz")
	}
	if err := compress.Close(); err != nil {
		return nil, nil, 0, errors.Wrap(err, "closing data.tar.gz")
	}

	return buf.Bytes(), md5buf.Bytes(), instSize, nil
}

var controlTemplate = `Package: {{.Info.Name}}
Version: {{.Info.Version}}
Section: {{.Info.Section}}
Priority: {{.Info.Priority}}
Architecture: {{.Info.Arch}}
Maintainer: {{.Info.Maintainer}}
Vendor: {{.Info.Vendor}}
Installed-Size: {{.InstalledSize}}
Replaces: {{join .Info.Replaces}}
Provides: {{join .Info.Provides}}
Depends: {{join .Info.Depends}}
Conflicts: {{join .Info.Conflicts}}
Homepage: {{.Info.Homepage}}
Description: {{.Info.Description}}
`

type controlData struct {
	Info          nfpm.Info
	InstalledSize int64
}

func createControl(now time.Time, instSize int64, md5sums []byte, info nfpm.Info) (controlTarGz []byte, err error) {
	var buf bytes.Buffer
	var compress = gzip.NewWriter(&buf)
	var out = tar.NewWriter(compress)
	defer out.Close()
	defer compress.Close()

	var body bytes.Buffer
	var tmpl = template.New("control")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
	})
	if err := template.Must(tmpl.Parse(controlTemplate)).Execute(&body, controlData{
		Info:          info,
		InstalledSize: instSize / 1024,
	}); err != nil {
		return nil, err
	}
	var header = tar.Header{
		Name:     "control",
		Size:     int64(body.Len()),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := out.WriteHeader(&header); err != nil {
		return nil, errors.Wrap(err, "cannot write header of control file to control.tar.gz")
	}
	if _, err := out.Write(body.Bytes()); err != nil {
		return nil, errors.Wrap(err, "cannot write control file to control.tar.gz")
	}

	header = tar.Header{
		Name:     "md5sums",
		Size:     int64(len(md5sums)),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := out.WriteHeader(&header); err != nil {
		return nil, errors.Wrap(err, "cannot write header of md5sums file to control.tar.gz")
	}
	if _, err := out.Write(md5sums); err != nil {
		return nil, errors.Wrap(err, "cannot write md5sums file to control.tar.gz")
	}

	if err := out.Close(); err != nil {
		return nil, errors.Wrap(err, "closing control.tar.gz")
	}
	if err := compress.Close(); err != nil {
		return nil, errors.Wrap(err, "closing control.tar.gz")
	}
	return buf.Bytes(), nil
}
