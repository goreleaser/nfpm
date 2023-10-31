package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

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
			if err := os.WriteFile(root.config, []byte(example), 0o666); err != nil {
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

const example = `# nfpm example configuration file
#
# check https://nfpm.goreleaser.com/configuration for detailed usage
#
name: "foo"
arch: "amd64"
platform: "linux"
version: "1.0.0"
section: "default"
priority: "extra"
replaces:
- foobar
provides:
- bar
depends:
- foo
- bar
recommends:
- whatever
suggests:
- something-else
conflicts:
- not-foo
- not-bar
maintainer: "John Doe <john@example.com>"
description: |
  FooBar is the great foo and bar software.
    And this can be in multiple lines!
vendor: "FooBarCorp"
homepage: "http://example.com"
license: "MIT"
changelog: "changelog.yaml"
contents:
- src: ./foo
  dst: /usr/bin/foo
- src: ./bar
  dst: /usr/bin/bar
- src: ./foobar.conf
  dst: /etc/foobar.conf
  type: config
- src: /usr/bin/foo
  dst: /sbin/foo
  type: symlink
overrides:
  rpm:
    scripts:
      preinstall: ./scripts/preinstall.sh
      postremove: ./scripts/postremove.sh
  deb:
    scripts:
      postinstall: ./scripts/postinstall.sh
      preremove: ./scripts/preremove.sh
`
