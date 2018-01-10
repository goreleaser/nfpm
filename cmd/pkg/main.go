package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/alecthomas/kingpin"
	"github.com/caarlos0/pkg"
	"github.com/caarlos0/pkg/deb"
)

var (
	app    = kingpin.New("pkg", "packages apps")
	config = app.Flag("config", "config file").ExistingFile()
	format = app.Flag("format", "format to package").Default("deb").String()
	target = app.Flag("target", "where to save the package").Required().String()
)

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bts, err := ioutil.ReadFile(*config)
	kingpin.FatalIfError(err, "")

	var info pkg.Info
	kingpin.FatalIfError(yaml.Unmarshal(bts, &info), "%v")

	var packager pkg.Packager
	switch *format {
	case "deb":
		packager = deb.Default
	}

	if packager == nil {
		kingpin.Fatalf("format %s is not implemented yet", *format)
	}

	f, err := os.Create(*target)
	kingpin.FatalIfError(err, "")
	kingpin.FatalIfError(packager.Package(ctx, info, f), "")
	fmt.Println("done:", *target)
}
