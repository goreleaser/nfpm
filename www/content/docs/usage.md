---
title: Usage
weight: 1
---

nFPM can be used both as a command line tool or as a Go library.

## Getting Started

{{% steps %}}

### Install nFPM

Choose your preferred installation method:

**Using Homebrew (recommended):**

```sh
brew install goreleaser/tap/nfpm
```

**Using Go:**

```sh
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
```

**Download Binary:** Get the latest from the [releases page](https://github.com/goreleaser/nfpm/releases).

### Initialize your project

Use [`nfpm init`](/docs/cmd/nfpm_init) to create a sample configuration:

```sh
nfpm init
```

This creates a `nfpm.yaml` file with a commented example configuration.

### Build your packages

Use [`nfpm package`](/docs/cmd/nfpm_package) to create your packages:

```sh
# Build specific formats
nfpm pkg --packager deb --target /tmp/
nfpm pkg --packager rpm --target /tmp/
nfpm pkg --packager apk --target /tmp/
```

You can also use `ipk` and `archlinux` as packagers.

{{% /steps %}}

## Command Line Reference

For more information about available options:

```sh
nfpm --help
```

See the [configuration reference](/docs/configuration) to customize your package definition.

Check out the [command line reference](/docs/cmd) for detailed documentation of all commands.

## Using as a Go library

You can also use nFPM as a library in your Go project.

Check out the [GoDocs page](https://pkg.go.dev/github.com/goreleaser/nfpm/v2?tab=doc),
the [nFPM command line implementation](https://github.com/goreleaser/nfpm/blob/main/cmd/nfpm/main.go)
and [GoReleaser's usage](https://github.com/goreleaser/goreleaser/blob/main/internal/pipe/nfpm/nfpm.go).
