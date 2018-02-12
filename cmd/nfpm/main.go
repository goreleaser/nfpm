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
	app    = kingpin.New("pkg", "packages apps")
	config = app.Flag("config", "config file").ExistingFile()
	format = app.Flag("format", "format to package").Default("deb").String()
	target = app.Flag("target", "where to save the package").Required().String()
)

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))

	bts, err := ioutil.ReadFile(*config)
	kingpin.FatalIfError(err, "")

	var info nfpm.Info
	kingpin.FatalIfError(yaml.Unmarshal(bts, &info), "%v")

	var pkg = nfpm.Get(*format)
	if pkg == nil {
		kingpin.Fatalf("format %s is not implemented yet", *format)
	}

	f, err := os.Create(*target)
	kingpin.FatalIfError(err, "")
	kingpin.FatalIfError(pkg.Package(info, f), "")
	fmt.Println("done:", *target)
}
