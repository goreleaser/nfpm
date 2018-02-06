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
)

var _ nfpm.Packager = Default

// Default deb packager
var Default = &RPM{}

// RPM is a RPM packager implementation
type RPM struct{}

// Package writes a new RPM package to the given writer using the given info
func (*RPM) Package(info nfpm.Info, w io.Writer) error {
	if info.Arch == "amd64" {
		info.Arch = "x86_64"
	}
	if info.Platform == "" {
		info.Platform = "linux"
	}

	root, err := ioutil.TempDir("", info.Name)
	if err != nil {
		return err
	}
	if err := createDirs(root); err != nil {
		return err
	}

	folder := fmt.Sprintf("%s-%s", info.Name, info.Version)
	bts, err := createTarGz(info, folder)
	if err != nil {
		return err
	}
	targzPath := filepath.Join(root, "SOURCES", folder+".tar.gz")
	targz, err := os.Create(targzPath)
	if err != nil {
		return fmt.Errorf("failed to create tar.gz file: %s", err)
	}
	defer targz.Close()
	if _, err := targz.Write(bts); err != nil {
		return err
	}
	if err := targz.Close(); err != nil {
		return err
	}

	specPath := filepath.Join(root, "SPECS", info.Name+".spec")
	if err := createSpec(info, specPath); err != nil {
		return err
	}

	var args = []string{
		"--define", fmt.Sprintf("_topdir %s", root),
		"--define", fmt.Sprintf("_tmppath %s/tmp", root),
		"--target", fmt.Sprintf("%s-unknown-%s", info.Arch, info.Platform),
		"-ba",
		"SPECS/" + info.Name + ".spec",
	}
	cmd := exec.Command("rpmbuild", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rpmbuild failed: %s", string(out))
	}

	rpmPath := filepath.Join(
		root, "RPMS", info.Arch,
		fmt.Sprintf("%s-%s-1.%s.rpm", info.Name, info.Version, info.Arch),
	)
	rpm, err := os.Open(rpmPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rpm)
	return err
}

func createSpec(info nfpm.Info, path string) error {
	var body bytes.Buffer
	var tmpl = template.New("spec")
	tmpl.Funcs(template.FuncMap{
		"join": func(strs []string) string {
			return strings.Trim(strings.Join(strs, ", "), " ")
		},
		"first_line": func(str string) string {
			return strings.Split(str, "\n")[0]
		},
	})
	if err := template.Must(tmpl.Parse(specTemplate)).Execute(&body, info); err != nil {
		return fmt.Errorf("failed to create spec file: %s", err)
	}
	if err := ioutil.WriteFile(path, body.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write spec file: %s", err)
	}
	return nil
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
		if err := os.Mkdir(path, 0744); err != nil {
			return fmt.Errorf("could not create dir %s: %s", path, err)
		}
	}
	return nil
}

func createTarGz(info nfpm.Info, root string) ([]byte, error) {
	var buf bytes.Buffer
	var compress = gzip.NewWriter(&buf)
	var out = tar.NewWriter(compress)
	defer out.Close()
	defer compress.Close()

	for _, files := range []map[string]string{info.Files, info.ConfigFiles} {
		for src, dst := range files {
			file, err := os.Open(src)
			if err != nil {
				return nil, fmt.Errorf("cannot open %s: %v", src, err)
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil || info.IsDir() {
				continue
			}
			var header = tar.Header{
				Name:    filepath.Join(root, dst),
				Size:    info.Size(),
				Mode:    int64(info.Mode()),
				ModTime: info.ModTime(),
			}
			if err := out.WriteHeader(&header); err != nil {
				return nil, fmt.Errorf("cannot write header of %s to data.tar.gz: %v", header.Name, err)
			}
			if _, err := io.Copy(out, file); err != nil {
				return nil, fmt.Errorf("cannot write %s to data.tar.gz: %v", header.Name, err)
			}
		}
	}

	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("closing data.tar.gz: %v", err)
	}
	if err := compress.Close(); err != nil {
		return nil, fmt.Errorf("closing data.tar.gz: %v", err)
	}

	return buf.Bytes(), nil
}

const specTemplate = `
%define __spec_install_post %{nil}
%define debug_package %{nil}
%define __os_install_post %{_dbpath}/brp-compress
%define _arch {{.Arch}}

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
Obsolotes: {{ . }}
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
#%config(noreplace) {{ . }}
{{ end }}

%changelog
# noop
`
