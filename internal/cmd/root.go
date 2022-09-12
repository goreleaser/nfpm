package cmd

import (
	"fmt"

	_ "github.com/goreleaser/nfpm/v2/apk"
	_ "github.com/goreleaser/nfpm/v2/arch"
	_ "github.com/goreleaser/nfpm/v2/deb"
	_ "github.com/goreleaser/nfpm/v2/rpm"
	"github.com/spf13/cobra"
)

func Execute(version string, exit func(int), args []string) {
	newRootCmd(version, exit).Execute(args)
}

type rootCmd struct {
	cmd  *cobra.Command
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
	cmd := &cobra.Command{
		Use:           "nfpm",
		Short:         "Packages apps on RPM, Deb and APK formats based on a YAML configuration file",
		Long:          `nFPM is a simple, 0-dependencies, deb, rpm and apk packager.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
	}

	cmd.AddCommand(
		newInitCmd().cmd,
		newPackageCmd().cmd,
		newDocsCmd().cmd,
		newManCmd().cmd,
		newSchemaCmd().cmd,
	)

	root.cmd = cmd
	return root
}
