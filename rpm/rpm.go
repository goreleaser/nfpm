package rpm

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alecthomas/template"
	"github.com/goreleaser/archive"
	"github.com/goreleaser/packager"
)

var _ packager.Packager = Default

// Default deb packager
var Default = &RPM{}

// RPM is a RPM packager implementation
type RPM struct{}

// Package writes a new RPM package to the given writer using the given info
func (*RPM) Package(info packager.Info, w io.Writer) error {
	if info.Arch == "amd64" {
		info.Arch = "x86_64"
	}
	root, err := ioutil.TempDir("", info.Name)
	if err != nil {
		return err
	}
	for _, folder := range []string{
		"RPMS",
		"SRPMS",
		"BUILD",
		"SOURCES",
		"SPECS",
		"tmp",
	} {
		if err := os.Mkdir(filepath.Join(root, folder), 0744); err != nil {
			return err
		}
	}
	targz, err := os.Create(filepath.Join(root, "SOURCES", fmt.Sprintf("%s-%s.tar.gz", info.Name, info.Version)))
	if err != nil {
		return fmt.Errorf("failed to create tar.gz file: %s", err)
	}
	archive := archive.New(targz)
	defer archive.Close()
	for src, dst := range info.Files {
		if err := archive.Add(fmt.Sprintf("%s-%s/%s", info.Name, info.Version, dst), src); err != nil {
			return fmt.Errorf("failed to add file %s to tar.gz: %s", src, err)
		}
	}
	archive.Close()

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
	if err := ioutil.WriteFile(filepath.Join(root, "SPECS", info.Name+".spec"), body.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write spec file: %s", err)
	}

	var args = []string{
		"--define", fmt.Sprintf("_topdir %s", root),
		"--define", fmt.Sprintf("_tmppath %s/tmp", root),
		"--target", fmt.Sprintf("%s-unknown-%s", info.Arch, info.Platform),
		"-ba",
		"SPECS/" + info.Name + ".spec",
	}
	log.Println(args)
	cmd := exec.Command("rpmbuild", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	log.Println(string(out))
	rpm, err := os.Open(filepath.Join(
		root,
		"RPMS",
		info.Arch,
		fmt.Sprintf("%s-%s-1.%s.rpm", info.Name, info.Version, info.Arch),
	))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, rpm)
	return err
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
{{ range $index, $element := .ConfigFiles }}
#%config(noreplace) {{ . }}
{{ end }}
{{ range $index, $element := .Files }}
{{ . }}
{{ end }}
%{_bindir}/*

%changelog
# noop
`
