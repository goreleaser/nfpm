package cmd

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

//go:embed example.yml
var example []byte

type initCmd struct {
	cmd    *cobra.Command
	config string
}

func newInitCmd() *initCmd {
	root := &initCmd{}
	cmd := &cobra.Command{
		Use:               "init",
		Aliases:           []string{"i"},
		Short:             "Creates a sample nfpm.yaml configuration file",
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := os.WriteFile(root.config, example, 0o666); err != nil {
				return fmt.Errorf("failed to create example file: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&root.config, "config", "f", "nfpm.yaml", "path to the to-be-created config file")
	_ = cmd.MarkFlagFilename("config", "yaml", "yml")

	root.cmd = cmd
	return root
}
