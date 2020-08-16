# Usage

## Command Line

To create a sample config file, run:

```console
$ nfpm init
```

You can then customize it and package to the formats you want:

```console
$ nfpm pkg --packager deb --target /tmp/
using deb packager...
created package: /tmp/foo_1.0.0_amd64.deb

$ nfpm pkg --packager rpm --target /tmp/
using rpm packager...
created package: /tmp/foo-1.0.0.x86_64.rpm
```

## Go Library

Check the [GoDocs](https://pkg.go.dev/github.com/goreleaser/nfpm?tab=doc) page,
as well as [NFPM command line implementation](https://github.com/goreleaser/nfpm/blob/master/cmd/nfpm/main.go)
and [GoReleaser's usage](https://github.com/goreleaser/goreleaser/blob/master/internal/pipe/nfpm/nfpm.go).
