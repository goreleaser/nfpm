package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

type docsCmd struct {
	cmd *cobra.Command
}

func newDocsCmd() *docsCmd {
	root := &docsCmd{}
	cmd := &cobra.Command{
		Use:                   "docs",
		Short:                 "Generates nFPM's command line docs",
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		Hidden:                true,
		Args:                  cobra.NoArgs,
		ValidArgsFunction:     cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			root.cmd.Root().DisableAutoGenTag = true
			return doc.GenMarkdownTreeCustom(root.cmd.Root(), "www/content/docs/cmd", func(_ string) string {
				return ""
			}, func(s string) string {
				return "/docs/cmd/" + strings.TrimSuffix(s, ".md") + "/"
			})
		},
	}

	root.cmd = cmd
	return root
}
