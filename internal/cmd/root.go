package cmd

import (
	"context"
	"os"

	goversion "github.com/caarlos0/go-version"
	"github.com/charmbracelet/fang"
	_ "github.com/goreleaser/nfpm/v2/apk"  // apk packager
	_ "github.com/goreleaser/nfpm/v2/app"  // macOS app packager
	_ "github.com/goreleaser/nfpm/v2/arch" // archlinux packager
	_ "github.com/goreleaser/nfpm/v2/deb"  // deb packager
	_ "github.com/goreleaser/nfpm/v2/dmg"  // macOS dmg packager
	_ "github.com/goreleaser/nfpm/v2/ipk"  // ipk packager
	_ "github.com/goreleaser/nfpm/v2/msi"  // msi packager
	_ "github.com/goreleaser/nfpm/v2/msix" // msix packager
	_ "github.com/goreleaser/nfpm/v2/pkg"  // macOS pkg packager
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

	if err := fang.Execute(
		context.Background(),
		cmd.cmd,
		fang.WithVersion(cmd.cmd.Version),
		fang.WithColorSchemeFunc(fang.AnsiColorScheme),
		fang.WithNotifySignal(os.Interrupt, os.Kill),
		fang.WithoutVersion(),
	); err != nil {
		cmd.exit(1)
	}
}

func newRootCmd(version goversion.Info, exit func(int)) *rootCmd {
	root := &rootCmd{
		exit: exit,
	}
	cmd := &cobra.Command{
		Use:               "nfpm",
		Short:             "Packages apps on RPM, Deb, APK, Arch Linux, ipk, MSI, MSIX, and macOS (app, dmg, pkg) formats based on a YAML configuration file",
		Long:              `nFPM is a simple and 0-dependencies apk, app, arch, deb, dmg, ipk, msi, msix, pkg, and rpm packager written in Go.`,
		Version:           version.String(),
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
	}
	cmd.SetVersionTemplate("{{.Version}}")

	cmd.AddCommand(
		newInitCmd().cmd,
		newPackageCmd().cmd,
		newDocsCmd().cmd,
		newSchemaCmd().cmd,
	)

	root.cmd = cmd
	return root
}
