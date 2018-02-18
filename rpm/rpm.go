package rpm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/goreleaser/nfpm"
	"github.com/pkg/errors"
)

func init() {
	nfpm.Register("rpm", Default)
}

// Default deb packager
var Default = &RPM{}

// RPM is a RPM packager implementation
type RPM struct{}

// Package writes a new RPM package to the given writer using the given info
func (*RPM) Package(info nfpm.Info, w io.Writer) error {
	if info.Arch == "amd64" {
		info.Arch = "x86_64"
	}
	temps, err := setupTempFiles(info)
	if err != nil {
		return err
	}
	if err = createTarGz(info, temps.Folder, temps.Source); err != nil {
		return errors.Wrap(err, "failed to create tar.gz")
	}
	if err = createSpec(info, temps.Spec); err != nil {
		return errors.Wrap(err, "failed to create rpm spec file")
	}

	var args = []string{
		"--define", fmt.Sprintf("_topdir %s", temps.Root),
		"--define", fmt.Sprintf("_tmppath %s/tmp", temps.Root),
		"--target", fmt.Sprintf("%s-unknown-%s", info.Arch, info.Platform),
		"-ba",
		"SPECS/" + info.Name + ".spec",
	}
	// #nosec
	cmd := exec.Command("rpmbuild", args...)
	cmd.Dir = temps.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		var msg = "rpmbuild failed"
		if string(out) != "" {
			msg += ": " + string(out)
		}
		return errors.Wrap(err, msg)
	}

	rpm, err := os.Open(temps.RPM)
	if err != nil {
		return errors.Wrap(err, "failed open rpm file")
	}
	_, err = io.Copy(w, rpm)
	return errors.Wrap(err, "failed to copy rpm file to writer")
}

func createSpec(info nfpm.Info, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0655)
	if err != nil {
		return errors.Wrap(err, "failed to create spec")
	}
	return writeSpec(file, info)
}

func writeSpec(w io.Writer, info nfpm.Info) error {
	var tmpl = template.New("spec")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"first_line": func(str string) string {
			return strings.Split(str, "\n")[0]
		},
	})
	if err := template.Must(tmpl.Parse(specTemplate)).Execute(w, info); err != nil {
		return errors.Wrap(err, "failed to parse spec template")
	}
	return nil
}

type tempFiles struct {
	// Root folder - topdir on rpm's slang
	Root string
	// Folder is the name of subfolders and etc, in the `name-version` format
	Folder string
	// Source is the path the .tar.gz file should be in
	Source string
	// Spec is the path the .spec file should be in
	Spec string
	// RPM is the path where the .rpm file should be generated
	RPM string
}

func setupTempFiles(info nfpm.Info) (tempFiles, error) {
	root, err := ioutil.TempDir("", info.Name)
	if err != nil {
		return tempFiles{}, errors.Wrap(err, "failed to create temp dir")
	}
	if err := createDirs(root); err != nil {
		return tempFiles{}, errors.Wrap(err, "failed to rpm dir structure")
	}
	folder := fmt.Sprintf("%s-%s", info.Name, info.Version)
	return tempFiles{
		Root:   root,
		Folder: folder,
		Source: filepath.Join(root, "SOURCES", folder+".tar.gz"),
		Spec:   filepath.Join(root, "SPECS", info.Name+".spec"),
		RPM:    filepath.Join(root, "RPMS", info.Arch, fmt.Sprintf("%s-1.%s.rpm", folder, info.Arch)),
	}, nil
}

func createDirs(root string) error {
	for _, folder := range []string{
		"RPMS",
		"SRPMS",
		"BUILD",
		"SOURCES",
		"SPECS",
		"tmp",
	} {
		path := filepath.Join(root, folder)
		if err := os.Mkdir(path, 0700); err != nil {
			return errors.Wrapf(err, "failed to create %s", path)
		}
	}
	return nil
}

func createTarGz(info nfpm.Info, root, file string) error {
	var buf bytes.Buffer
	var compress = gzip.NewWriter(&buf)
	var out = tar.NewWriter(compress)
	// the writers are properly closed later, this is just in case that we have
	// an error in another part of the code.
	defer out.Close()      // nolint: errcheck
	defer compress.Close() // nolint: errcheck

	for _, files := range []map[string]string{info.Files, info.ConfigFiles} {
		for src, dst := range files {
			if err := copyToTarGz(out, root, src, dst); err != nil {
				return err
			}
		}
	}
	if err := out.Close(); err != nil {
		return errors.Wrap(err, "failed to close data.tar.gz writer")
	}
	if err := compress.Close(); err != nil {
		return errors.Wrap(err, "failed to close data.tar.gz gzip writer")
	}
	if err := ioutil.WriteFile(file, buf.Bytes(), 0666); err != nil {
		return errors.Wrap(err, "could not write to .tar.gz file")
	}
	return nil
}

func copyToTarGz(out *tar.Writer, root, src, dst string) error {
	file, err := os.OpenFile(src, os.O_RDONLY, 0600)
	if err != nil {
		return errors.Wrap(err, "could not add file to the archive")
	}
	// don't really care if Close() errs
	defer file.Close() // nolint: errcheck
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	var header = tar.Header{
		Name:    filepath.Join(root, dst),
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}
	if err := out.WriteHeader(&header); err != nil {
		return errors.Wrapf(err, "cannot write header of %s to data.tar.gz", header.Name)
	}
	if _, err := io.Copy(out, file); err != nil {
		return errors.Wrapf(err, "cannot write %s to data.tar.gz", header.Name)
	}
	return nil
}

const specTemplate = `
%define __spec_install_post %{nil}
%define debug_package %{nil}
%define __os_install_post %{_dbpath}/brp-compress
%define _arch {{.Arch}}
%define _bindir {{.Bindir}}

Name: {{ .Name }}
Summary: {{ first_line .Description }}
Version: {{ .Version }}
Release: 1
License: {{ .License }}
Group: Development/Tools
SOURCE0 : %{name}-%{version}.tar.gz
URL: {{ .Homepage }}
BuildRoot: %{_tmppath}/%{name}-%{version}-%{release}-root

{{ range $index, $element := .Replaces }}
Obsoletes: {{ . }}
{{ end }}

{{ range $index, $element := .Conflicts }}
Conflicts: {{ . }}
{{ end }}

{{ range $index, $element := .Provides }}
Provides: {{ . }}
{{ end }}

{{ range $index, $element := .Depends }}
Requires: {{ . }}
{{ end }}

{{ range $index, $element := .Recommends }}
Recommends: {{ . }}
{{ end }}

%description
{{ .Description }}

%prep
%setup -q

%build
# Empty section.

%install
rm -rf %{buildroot}
mkdir -p  %{buildroot}

# in builddir
cp -a * %{buildroot}

%clean
rm -rf %{buildroot}

%files
%defattr(-,root,root,-)
{{ range $index, $element := .Files }}
{{ . }}
{{ end }}
%{_bindir}/*
{{ range $index, $element := .ConfigFiles }}
{{ . }}
{{ end }}
{{ range $index, $element := .ConfigFiles }}
%config(noreplace) {{ . }}
{{ end }}

%changelog
# noop
`
