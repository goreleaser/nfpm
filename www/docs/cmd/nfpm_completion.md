# nfpm completion

Prints shell autocompletion scripts for nFPM

## Synopsis

Allows you to setup your shell to completions nFPM commands and flags.

### Bash

	$ source <(nfpm completion bash)

To load completions for each session, execute once:

#### Linux

	$ nfpm completion bash > /etc/bash_completion.d/nfpm

#### MacOS

	$ nfpm completion bash > /usr/local/etc/bash_completion.d/nfpm

### ZSH

If shell completion is not already enabled in your environment you will need to enable it.
You can execute the following once:

	$ echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions for each session, execute once:

	$ nfpm completion zsh > "${fpath[1]}/_nfpm"

You will need to start a new shell for this setup to take effect.

### Fish

	$ nfpm completion fish | source

To load completions for each session, execute once:

	$ nfpm completion fish > ~/.config/fish/completions/nfpm.fish

**NOTE**: If you are using an official nfpm package, it should setup completions for you out of the box.


```
nfpm completion [bash|zsh|fish]
```

## Options

```
  -h, --help   help for completion
```

## See also

* [nfpm](/cmd/nfpm/)	 - Packages apps on RPM, Deb and APK formats based on a YAML configuration file

