// Package deb implements pkg.Packager providing .deb bindings.
package deb

import (
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/caarlos0/pkg"
	"github.com/caarlos0/pkg/tmpl"
)

// Deb is a deb pkg
type Deb struct {
	ctx  context.Context
	Info pkg.Info
	Path string
	Tmp  string
}

// New deb package with the given ctx and info
func New(ctx context.Context, info pkg.Info, path string) (*Deb, error) {
	folder, err := ioutil.TempDir("", "deb")
	if err != nil {
		return nil, err
	}
	log.Println("creating", folder)
	return &Deb{
		ctx:  ctx,
		Info: info,
		Tmp:  folder,
		Path: path,
	}, nil
}

// Add adds a file to the package
func (d *Deb) Add(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(d.Tmp, filepath.Dir(dst)), 0755); err != nil {
		return err
	}
	bts, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filepath.Join(d.Tmp, dst), bts, info.Mode())
}

// Close closes the package
func (d *Deb) Close() error {
	if err := d.createControl(); err != nil {
		return err
	}
	cmd := exec.CommandContext(d.ctx, "dpkg-deb", "--root-owner-group", "--build", d.Tmp, d.Path)
	bts, err := cmd.CombinedOutput()
	log.Println(string(bts))
	return err
}

var controlTemplate = `Package: {{.Name}}
Version: {{.Version}}
Section: {{.Section}}
Priority: {{.Priority}}
Architecture: {{.Arch}}
Depends: {{ join .Depends }}
Maintainer: {{.Maintainer}}
Vendor: {{.Vendor}}
Homepage: {{.Homepage}}
Description: {{.Description}}
`

func (d *Deb) createControl() error {
	var b bytes.Buffer
	t := template.New("control")
	t.Funcs(template.FuncMap{
		"join": tmpl.Join,
	})
	if err := template.Must(t.Parse(controlTemplate)).Execute(&b, d.Info); err != nil {
		return err
	}
	control := filepath.Join(d.Tmp, "DEBIAN", "control")
	if err := os.Mkdir(filepath.Dir(control), 0755); err != nil {
		return err
	}
	return ioutil.WriteFile(control, b.Bytes(), 0644)
}
