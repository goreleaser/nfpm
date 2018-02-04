package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/alecthomas/kingpin"
	"github.com/goreleaser/packager"
	"github.com/goreleaser/packager/deb"
	"github.com/goreleaser/packager/rpm"
	yaml "gopkg.in/yaml.v1"
)

var (
	app    = kingpin.New("pkg", "packages apps")
	config = app.Flag("config", "config file").ExistingFile()
	format = app.Flag("format", "format to package").Default("deb").String()
	target = app.Flag("target", "where to save the package").Required().String()
)

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))

	bts, err := ioutil.ReadFile(*config)
	kingpin.FatalIfError(err, "")

	var info packager.Info
	kingpin.FatalIfError(yaml.Unmarshal(bts, &info), "%v")

	var packager packager.Packager
	switch *format {
	case "deb":
		packager = deb.Default
	case "rpm":
		packager = rpm.Default
	}

	if packager == nil {
		kingpin.Fatalf("format %s is not implemented yet", *format)
	}

	f, err := os.Create(*target)
	kingpin.FatalIfError(err, "")
	kingpin.FatalIfError(packager.Package(info, f), "")
	fmt.Println("done:", *target)
}
