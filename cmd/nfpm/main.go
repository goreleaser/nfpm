// Package main contains the main nfpm cli source code.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
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
	target = pkgCmd.Flag("target", "where to save the generated package").
		Default("/tmp/foo.deb").
		Short('t').
		String()

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
		if err := doPackage(*config, *target); err != nil {
			kingpin.Fatalf(err.Error())
		}
		fmt.Printf("created package: %s\n", *target)
	}
}

func initFile(config string) error {
	return ioutil.WriteFile(config, []byte(example), 0666)
}

func doPackage(path, target string) error {
	format := filepath.Ext(target)[1:]
	config, err := nfpm.ParseFile(path)
	if err != nil {
		return err
	}

	info, err := config.Get(format)
	if err != nil {
		return err
	}

	info = nfpm.WithDefaults(info)

	if err = nfpm.Validate(info); err != nil {
		return err
	}

	fmt.Printf("using %s packager...\n", format)
	pkg, err := nfpm.Get(format)
	if err != nil {
		return err
	}

	f, err := os.Create(target)
	if err != nil {
		return err
	}
	return pkg.Package(info, f)
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
bindir: "/usr/local/bin"
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
