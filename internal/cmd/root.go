package cmd

import (
	"fmt"

	_ "github.com/goreleaser/nfpm/v2/apk"
	_ "github.com/goreleaser/nfpm/v2/deb"
	_ "github.com/goreleaser/nfpm/v2/rpm"
	"github.com/muesli/coral"
)

func Execute(version string, exit func(int), args []string) {
	newRootCmd(version, exit).Execute(args)
}

type rootCmd struct {
	cmd  *coral.Command
	exit func(int)
}

func (cmd *rootCmd) Execute(args []string) {
	cmd.cmd.SetArgs(args)

	if err := cmd.cmd.Execute(); err != nil {
		fmt.Println(err.Error())
		cmd.exit(1)
	}
}

func newRootCmd(version string, exit func(int)) *rootCmd {
	root := &rootCmd{
		exit: exit,
	}
	cmd := &coral.Command{
		Use:           "nfpm",
		Short:         "Packages apps on RPM, Deb and APK formats based on a YAML configuration file",
		Long:          `nFPM is a simple, 0-dependencies, deb, rpm and apk packager.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          coral.NoArgs,
	}

	cmd.AddCommand(
		newInitCmd().cmd,
		newPackageCmd().cmd,
		newDocsCmd().cmd,
		newSchemaCmd().cmd,
	)

	root.cmd = cmd
	return root
}
