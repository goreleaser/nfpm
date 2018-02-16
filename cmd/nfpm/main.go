package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/alecthomas/kingpin"
	"github.com/goreleaser/nfpm"
	_ "github.com/goreleaser/nfpm/deb"
	_ "github.com/goreleaser/nfpm/rpm"
	yaml "gopkg.in/yaml.v1"
)

var (
	app    = kingpin.New("nfpm", "not-fpm packages apps in some formats")
	config = app.Flag("config", "config file").Default("nfpm.yaml").String()

	pkgCmd = app.Command("pkg", "package based on the config file")
	format = pkgCmd.Flag("format", "format to package").Default("deb").String()
	target = pkgCmd.Flag("target", "where to save the package").Required().String()

	initCmd = app.Command("init", "create an empty config file")
)

func main() {
	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case initCmd.FullCommand():
		if err := initFile(*config); err != nil {
			kingpin.Fatalf(err.Error())
		}
		fmt.Printf("created empty config file at %s, edit at will\n", *config)
	case pkgCmd.FullCommand():
		if err := doPackage(*config, *format, *target); err != nil {
			kingpin.Fatalf(err.Error())
		}
	}
}

func initFile(config string) error {
	yml, err := yaml.Marshal(nfpm.Info{})
	if err != nil {
		return err
	}
	return ioutil.WriteFile(config, yml, 0666)
}

func doPackage(config, format, target string) error {
	bts, err := ioutil.ReadFile(config)
	if err != nil {
		return err
	}

	var info nfpm.Info
	err = yaml.Unmarshal(bts, &info)
	if err != nil {
		return err
	}

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
