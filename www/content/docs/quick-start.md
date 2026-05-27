---
title: Quick Start
weight: 1
---

nFPM can be used both as a command line tool or as a Go library.

## Getting Started

{{% steps %}}

### Install nFPM

You can choose from [several instalation methods](/docs/install), for example:

**Using Homebrew:**

```sh
brew install goreleaser/tap/nfpm
```

**Using go install:**

```sh
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
```

Make sure to [check the complete list](/docs/install) and choose the best option
for your case.

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
nfpm pkg --packager xbps --target /tmp/
```

You can also use `ipk`, `archlinux`, `msix`, and `xbps` as packagers.

### Verify an XBPS package on Void Linux

nFPM generates XBPS packages natively. Compatibility is tested primarily against
Void Linux; other XBPS-based environments may also work, but they are secondary
targets.

In a disposable Void environment, a local repository smoke check can use the
standard XBPS tooling:

```sh
repo="$(mktemp -d)"
nfpm pkg --packager xbps --target "$repo/"
xbps-rindex -a "$repo"/*.xbps
xbps-install -i -R "$repo" -y foo
xbps-query -p pkgver foo
xbps-remove -y foo
```

When `xbps.signature.key_file` is configured, nFPM writes an adjacent
`<package>.xbps.sig2` sidecar for the generated package. This does not create or
sign repository metadata, publish repositories, manage remote repositories, or
orchestrate `xbps-rindex --sign`.

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
