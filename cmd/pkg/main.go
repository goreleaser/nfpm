package main

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/alecthomas/kingpin"
	"github.com/apex/log"
	"github.com/caarlos0/pkg"
	"github.com/caarlos0/pkg/deb"
)

var (
	app    = kingpin.New("pkg", "packages apps")
	config = app.Flag("config", "config file").ExistingFile()
	format = app.Flag("format", "format to package").Default("deb").String()
	files  = app.Flag("file", "file to add to the package, in the src=dst format").Required().Strings()
	target = app.Flag("target", "where to save the package").Required().String()
)

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	bts, err := ioutil.ReadFile(*config)
	kingpin.FatalIfError(err, "%v")

	var info pkg.Info
	kingpin.FatalIfError(yaml.Unmarshal(bts, &info), "%v")

	var packager pkg.Packager
	switch *format {
	case "deb":
		packager, err = deb.New(ctx, info, *target)
	default:
		log.Fatalf("packager not found for %s", *format)
	}
	kingpin.FatalIfError(err, "%v")

	for _, file := range *files {
		s := strings.Split(file, "=")
		kingpin.FatalIfError(packager.Add(s[0], s[1]), "%v")
	}

	kingpin.FatalIfError(packager.Close(), "%v")
}
