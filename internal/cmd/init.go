package cmd

import (
	"fmt"
	"os"

	"github.com/muesli/coral"
)

type initCmd struct {
	cmd    *coral.Command
	config string
}

func newInitCmd() *initCmd {
	root := &initCmd{}
	cmd := &coral.Command{
		Use:           "init",
		Aliases:       []string{"i"},
		Short:         "Creates a sample nfpm.yaml config file",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          coral.NoArgs,
		RunE: func(cmd *coral.Command, args []string) error {
			if err := os.WriteFile(root.config, []byte(example), 0o666); err != nil {
				return fmt.Errorf("failed to create example file: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&root.config, "config", "f", "nfpm.yaml", "path to the to-be-created config file")

	root.cmd = cmd
	return root
}

const example = `# nfpm example config file
#
# check https://nfpm.goreleaser.com/configuration for detailed usage
#
name: "foo"
arch: "amd64"
platform: "linux"
version: "v1.0.0"
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
  dst: /usr/local/bin/foo
- src: ./bar
  dst: /usr/local/bin/bar
- src: ./foobar.conf
  dst: /etc/foobar.conf
  type: config
- src: /usr/local/bin/foo
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
