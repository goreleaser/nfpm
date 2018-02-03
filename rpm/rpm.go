package rpm

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/goreleaser/archive"
	"github.com/goreleaser/packager"
)

var _ packager.Packager = Default

// Default deb packager
var Default = &RPM{}

// RPM is a RPM packager implementation
type RPM struct{}

// Package writes a new RPM package to the given writer using the given info
func (*RPM) Package(info packager.Info, deb io.Writer) error {
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
	targz, err := os.Create(filepath.Join(root, "SOURCES", info.Name+".tar.gz"))
	if err != nil {
		return fmt.Errorf("failed to create tar.gz file: %s", err)
	}
	archive := archive.New(targz)
	defer archive.Close()
	for src, dst := range info.Files {
		if err := archive.Add(dst, src); err != nil {
			return fmt.Errorf("failed to add file %s to tar.gz: %s", src, err)
		}
	}
	var args = []string{
		"--define", fmt.Sprintf("_topdir %s", root),
		"--define", fmt.Sprintf("_tmppath %s/tmp", root),
	}
	log.Println(args)
	return nil
}

const specTemplate = `
%define        __spec_install_post %{nil}
%define          debug_package %{nil}
%define        __os_install_post %{_dbpath}/brp-compress

Summary: {{.Info.Description}}
Name: {{.Info.Name}}
Version: {{.Info.Version}}
Release: 1
License: {{.Info.License}}
Group: Development/Tools
SOURCE0 : %{name}.tar.gz
URL: {{.Info.Homepage}}

BuildRoot: %{_tmppath}/%{name}-%{version}-%{release}-root

%description
%{summary}

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
%config(noreplace) %{_sysconfdir}/%{name}/%{name}.conf
%{_bindir}/*

`
