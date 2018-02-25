// Package deb implements nfpm.Packager providing .deb bindings.
package deb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	// #nosec
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

	// the writers are properly closed later, this is just in case that we have
	// an error in another part of the code.
	defer out.Close()      // nolint: errcheck
	defer compress.Close() // nolint: errcheck

	var md5buf bytes.Buffer
	for _, files := range []map[string]string{
		info.Files,
		info.ConfigFiles,
	} {
		for src, dst := range files {
			size, err := copyToTarAndDigest(out, &md5buf, now, src, dst)
			if err != nil {
				return nil, nil, 0, err
			}
			instSize += size
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

func copyToTarAndDigest(tarw *tar.Writer, md5w io.Writer, now time.Time, src, dst string) (int64, error) {
	file, err := os.OpenFile(src, os.O_RDONLY, 0600)
	if err != nil {
		return 0, errors.Wrap(err, "could not add file to the archive")
	}
	// don't care if it errs while closing...
	defer file.Close() // nolint: errcheck
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return 0, nil
	}
	var header = tar.Header{
		Name:    dst[1:],
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: now,
	}
	if err := tarw.WriteHeader(&header); err != nil {
		return 0, errors.Wrapf(err, "cannot write header of %s to data.tar.gz", header)
	}
	// #nosec
	var digest = md5.New()
	if _, err := io.Copy(tarw, io.TeeReader(file, digest)); err != nil {
		return 0, errors.Wrap(err, "failed to copy")
	}
	if _, err := fmt.Fprintf(md5w, "%x  %s\n", digest.Sum(nil), header.Name); err != nil {
		return 0, errors.Wrap(err, "failed to write md5")
	}
	return info.Size(), nil
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
Recommends: {{join .Info.Recommends}}
Suggests: {{join .Info.Suggests}}
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
	// the writers are properly closed later, this is just in case that we have
	// an error in another part of the code.
	defer out.Close()      // nolint: errcheck
	defer compress.Close() // nolint: errcheck

	var body bytes.Buffer
	if err := writeControl(&body, controlData{
		Info:          info,
		InstalledSize: instSize / 1024,
	}); err != nil {
		return nil, err
	}

	for name, content := range map[string][]byte{
		"control":   body.Bytes(),
		"md5sums":   md5sums,
		"conffiles": conffiles(info),
	} {
		if err := newFileInsideTarGz(out, name, content, now); err != nil {
			return nil, err
		}
	}

	if err := out.Close(); err != nil {
		return nil, errors.Wrap(err, "closing control.tar.gz")
	}
	if err := compress.Close(); err != nil {
		return nil, errors.Wrap(err, "closing control.tar.gz")
	}
	return buf.Bytes(), nil
}

func writeControl(w io.Writer, data controlData) error {
	var tmpl = template.New("control")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
	})
	return template.Must(tmpl.Parse(controlTemplate)).Execute(w, data)
}

func newFileInsideTarGz(out *tar.Writer, name string, content []byte, now time.Time) error {
	var header = tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := out.WriteHeader(&header); err != nil {
		return errors.Wrapf(err, "cannot write header of %s file to control.tar.gz", name)
	}
	if _, err := out.Write(content); err != nil {
		return errors.Wrapf(err, "cannot write %s file to control.tar.gz", name)
	}
	return nil
}

func conffiles(info nfpm.Info) []byte {
	var confs []string
	for _, dst := range info.ConfigFiles {
		confs = append(confs, dst)
	}
	return []byte(strings.Join(confs, "\n") + "\n")
}
