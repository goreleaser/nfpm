# Install

You can install the pre-compiled binary (in several different ways),
use Docker or compile from source.

Here are the steps for each of them:

## Install the pre-compiled binary

**homebrew tap** (only on macOS for now):

```console
$ brew install goreleaser/tap/nfpm
```

**scoop**:

```console
$ scoop bucket add goreleaser https://github.com/goreleaser/scoop-bucket.git
$ scoop install nfpm
```

**deb/rpm**:

Download the `.deb` or `.rpm` from the [releases page][releases] and
install with `dpkg -i` and `rpm -i` respectively.

**Shell script**:

```console
$ curl -sfL https://install.goreleaser.com/github.com/goreleaser/nfpm.sh | sh
```

**manually**:

Download the pre-compiled binaries from the [releases page][releases] and
copy to the desired location.

## Running with Docker

You can also use it within a Docker container. To do that, you'll need to
execute something more-or-less like the following:

```console
$ docker run --rm \
  -v $PWD:/tmp/pkg \
  goreleaser/nfpm pkg --config /tmp/pkg/foo.yml --target /tmp/pkg/foo.rpm
```

[releases]: https://github.com/goreleaser/nfpm/releases

## Compiling from source

Here you have two options:

If you want to contribute to the project, please follow the
steps on our [contributing guide](/contributing).

If you just want to build from source for whatever reason, follow these steps:

**Clone:**

```console
$ git clone https://github.com/goreleaser/nfpm
$ cd nfpm
```

**Get the dependencies:**

```console
$ go mod tidy
```

**Build:**

```console
$ go build -o nfpm cmd/nfpm/main.go
```

**Verify it works:**

```console
$ ./nfpm --version
```
