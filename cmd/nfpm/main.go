package main

import (
	"os"

	_ "embed"

	goversion "github.com/caarlos0/go-version"
	"github.com/goreleaser/nfpm/v2/internal/cmd"
)

// nolint: gochecknoglobals
var (
	version   = "dev"
	treeState = ""
	commit    = ""
	date      = ""
	builtBy   = ""
)

const website = "https://nfpm.goreleaser.com"

//go:embed art.txt
var asciiArt string

func main() {
	cmd.Execute(
		buildVersion(version, commit, date, builtBy, treeState),
		os.Exit,
		os.Args[1:],
	)
}

func buildVersion(version, commit, date, builtBy, treeState string) goversion.Info {
	return goversion.GetVersionInfo(
		goversion.WithAppDetails("nfpm", "a simple and 0-dependencies deb, rpm, apk and arch linux packager written in Go", website),
		goversion.WithASCIIName(asciiArt),
		func(i *goversion.Info) {
			if commit != "" {
				i.GitCommit = commit
			}
			if treeState != "" {
				i.GitTreeState = treeState
			}
			if date != "" {
				i.BuildDate = date
			}
			if version != "" {
				i.GitVersion = version
			}
			if builtBy != "" {
				i.BuiltBy = builtBy
			}
		},
	)
}
