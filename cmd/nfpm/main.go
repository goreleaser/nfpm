// Package main contains the main nfpm cli source code.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/alecthomas/kingpin"

	"github.com/goreleaser/nfpm"
	_ "github.com/goreleaser/nfpm/deb"
	_ "github.com/goreleaser/nfpm/rpm"
)

// nolint: gochecknoglobals
var (
	version = "master"

	app    = kingpin.New("nfpm", "not-fpm packages apps in some formats")
	config = app.Flag("config", "config file").
		Default("nfpm.yaml").
		Short('f').
		String()

	pkgCmd = app.Command("pkg", "package based on the config file").Alias("package")
	target = pkgCmd.Flag("target", "where to save the generated package (filename, folder or blank for current folder)").
		Default("").
		Short('t').
		String()
	packager = pkgCmd.Flag("packager", "which packager implementation to use").
			Short('p').
			Enum("deb", "rpm")

	initCmd = app.Command("init", "create an empty config file")
)

func main() {
	app.Version(version)
	app.VersionFlag.Short('v')
	app.HelpFlag.Short('h')
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case initCmd.FullCommand():
		if err := initFile(*config); err != nil {
			kingpin.Fatalf(err.Error())
		}
		fmt.Printf("created config file from example: %s\n", *config)
	case pkgCmd.FullCommand():
		if err := doPackage(*config, *target, *packager); err != nil {
			kingpin.Fatalf(err.Error())
		}
	}
}

func initFile(config string) error {
	return ioutil.WriteFile(config, []byte(example), 0600)
}

func doPackage(configPath, target, packager string) error {
	if packager == "" {
		fmt.Printf("guessing packager from target file extension...")
		packager = filepath.Ext(target)[1:]
	}
	config, err := nfpm.ParseFile(configPath)
	if err != nil {
		return err
	}

	info, err := config.Get(packager)
	if err != nil {
		return err
	}

	info = nfpm.WithDefaults(info)

	if err = nfpm.Validate(info); err != nil {
		return err
	}

	fmt.Printf("using %s packager...\n", packager)
	pkg, err := nfpm.Get(packager)
	if err != nil {
		return err
	}

	if target == "" {
		// if no target was specified create a package in
		// current directory with a conventional file name
		target = pkg.ConventionalFileName(info)
	} else {
		// if a directory was specified as target, create
		// a package with conventional file name there
		var stat os.FileInfo
		stat, err = os.Stat(target)
		if err == nil && stat.IsDir() {
			target = path.Join(target, pkg.ConventionalFileName(info))
		}
	}

	f, err := os.Create(target)
	if err != nil {
		return err
	}

	info.Target = target

	err = pkg.Package(info, f)
	_ = f.Close()
	if err != nil {
		os.Remove(target)
		return err
	}

	fmt.Printf("created package: %s\n", target)
	return nil
}

const example = `# nfpm example config file
name: "foo"
arch: "amd64"
platform: "linux"
version: "v${MY_APP_VERSION}"
section: "default"
priority: "extra"
replaces:
- foobar
provides:
- bar
depends:
- foo
- bar
# recommends on rpm packages requires rpmbuild >= 4.13
recommends:
- whatever
# suggests on rpm packages requires rpmbuild >= 4.13
suggests:
- something-else
conflicts:
- not-foo
- not-bar
maintainer: "John Doe <john@example.com>"
description: |
  FooBar is the great foo and bar software.
    And this can be in multiple lines!
vendor: "FooBarCorp"
homepage: "http://example.com"
license: "MIT"
files:
  ./foo: "/usr/local/bin/foo"
  ./bar: "/usr/local/bin/bar"
config_files:
  ./foobar.conf: "/etc/foobar.conf"
overrides:
  rpm:
    scripts:
      preinstall: ./scripts/preinstall.sh
      postremove: ./scripts/postremove.sh
  deb:
    scripts:
      postinstall: ./scripts/postinstall.sh
      preremove: ./scripts/preremove.sh
`
