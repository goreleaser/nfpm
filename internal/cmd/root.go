package cmd

import (
	"fmt"

	goversion "github.com/caarlos0/go-version"
	_ "github.com/goreleaser/nfpm/v2/apk"  // apk packager
	_ "github.com/goreleaser/nfpm/v2/arch" // archlinux packager
	_ "github.com/goreleaser/nfpm/v2/deb"  // deb packager
	_ "github.com/goreleaser/nfpm/v2/rpm"  // rpm packager
	"github.com/spf13/cobra"
)

func Execute(version goversion.Info, exit func(int), args []string) {
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

func newRootCmd(version goversion.Info, exit func(int)) *rootCmd {
	root := &rootCmd{
		exit: exit,
	}
	cmd := &cobra.Command{
		Use:               "nfpm",
		Short:             "Packages apps on RPM, Deb, APK and Arch Linux formats based on a YAML configuration file",
		Long:              `nFPM is a simple and 0-dependencies deb, rpm, apk and arch linux packager written in Go.`,
		Version:           version.String(),
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
	}
	cmd.SetVersionTemplate("{{.Version}}")

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
