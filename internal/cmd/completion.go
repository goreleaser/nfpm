package cmd


import "github.com/spf13/cobra"

type completionCmd struct {
	cmd *cobra.Command
}

func newCompletionCmd() *completionCmd {
	root := &completionCmd{}
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Prints shell autocompletion scripts for NFPM",
		Long: `Allows you to setup your shell to completions NFPM commands and flags.

#### Bash

	$ source <(nfpm completion bash)

To load completions for each session, execute once:

##### Linux

	$ nfpm completion bash > /etc/bash_completion.d/nfpm

##### MacOS

	$ nfpm completion bash > /usr/local/etc/bash_completion.d/nfpm

#### ZSH

If shell completion is not already enabled in your environment you will need to enable it.
You can execute the following once:

	$ echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions for each session, execute once:

	$ nfpm completion zsh > "${fpath[1]}/_nfpm"

You will need to start a new shell for this setup to take effect.

#### Fish

	$ nfpm completion fish | source

To load completions for each session, execute once:

	$ nfpm completion fish > ~/.config/fish/completions/nfpm.fish

**NOTE**: If you are using an official nfpm package, it should setup completions for you out of the box.
`,
		SilenceUsage:          true,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish"},
		Args:                  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			switch args[0] {
			case "bash":
				err = cmd.Root().GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				err = cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				err = cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			}

			return err
		},
	}

	root.cmd = cmd
	return root
}

