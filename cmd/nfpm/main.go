// Package main contains the main nfpm cli source code.
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/alecthomas/kingpin"
	"github.com/gobuffalo/packr"
	"github.com/goreleaser/nfpm"
	_ "github.com/goreleaser/nfpm/deb"
	_ "github.com/goreleaser/nfpm/rpm"
)

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
	box := packr.NewBox("./nfpm.yaml.example")
	return ioutil.WriteFile(config, box.Bytes("."), 0666)
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
	return pkg.Package(nfpm.WithDefaults(info), f)
}
