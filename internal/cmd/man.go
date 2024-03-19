package cmd

import (
	"fmt"
	"os"

	mcobra "github.com/muesli/mango-cobra"
	"github.com/muesli/roff"
	"github.com/spf13/cobra"
)

type manCmd struct {
	cmd *cobra.Command
}

func newManCmd() *manCmd {
	root := &manCmd{}
	cmd := &cobra.Command{
		Use:                   "man",
		Short:                 "Generates nfpm's command line manpages",
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		Hidden:                true,
		Args:                  cobra.NoArgs,
		ValidArgsFunction:     cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			manPage, err := mcobra.NewManPage(1, root.cmd.Root())
			if err != nil {
				return err
			}

			_, err = fmt.Fprint(os.Stdout, manPage.Build(roff.NewDocument()))
			return err
		},
	}

	root.cmd = cmd
	return root
}
