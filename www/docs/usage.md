# Usage

nFPM can be used both as command line tool or as a library.

## Command Line

To create a sample config file, run:

```sh
nfpm init
```

You can then customize it and package to the formats you want:

```sh
nfpm pkg --packager deb --target /tmp/
nfpm pkg --packager rpm --target /tmp/
```

You can learn about it in more detail in the [command line reference section](/cmd/nfpm/).

## Go Library

Check out the [GoDocs page](https://pkg.go.dev/github.com/goreleaser/nfpm/v2?tab=doc),
the [nFPM command line implementation](https://github.com/goreleaser/nfpm/blob/main/cmd/nfpm/main.go)
and [GoReleaser's usage](https://github.com/goreleaser/goreleaser/blob/main/internal/pipe/nfpm/nfpm.go).
