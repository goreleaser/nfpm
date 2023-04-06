# Install

You can install the pre-compiled binary (in several ways), use Docker
or compile from source.

Bellow you can find the steps for each of them.

## Install the pre-compiled binary

### homebrew tap

```bash
brew install goreleaser/tap/nfpm
```

### homebrew

```bash
brew install nfpm
```

!!! info
    The [formula in homebrew-core](https://github.com/Homebrew/homebrew-core/blob/master/Formula/nfpm.rb) might be slightly outdated.
    Use our homebrew tap to always get the latest updates.

### scoop

```bash
scoop bucket add goreleaser https://github.com/goreleaser/scoop-bucket.git
scoop install nfpm
```

### apt

```bash
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
sudo apt update
sudo apt install nfpm
```

### yum

```bash
echo '[goreleaser]
name=GoReleaser
baseurl=https://repo.goreleaser.com/yum/
enabled=1
gpgcheck=0' | sudo tee /etc/yum.repos.d/goreleaser.repo
sudo yum install nfpm
```

### deb, apk and rpm packages

Download the `.deb`, `.rpm` or `.apk` from the [releases page][releases] and
install them with the appropriate tools.

### go install

```bash
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
```

### manually

Download the pre-compiled binaries from the [releases page][releases] and copy
them to the desired location.

## Verifying the artifacts

### binaries

All artifacts are checksummed, and the checksum is signed with [cosign][].

1. Download the files you want, the `checksums.txt` and `checksums.txt.sig`
   files from the [releases][releases] page:
	```bash
	wget 'https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt'
	```

1. Verify the signature:
	```bash
	cosign verify-blob \
		--certificate-identity 'https://github.com/goreleaser/nfpm/.github/workflows/release.yml@refs/tags/__VERSION__' \
        --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
		--signature 'https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt' \
		--cert 'https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt.pem' \
		checksums.txt
	```
1. If the signature is valid, you can then verify the SHA256 sums match with the
   downloaded binary:
	```bash
	sha256sum --ignore-missing -c checksums.txt
	```

### docker images

Our Docker images are signed with [cosign][].

Verify the signature:

```bash
cosign verify goreleaser/nfpm
cosign verify ghcr.io/goreleaser/nfpm
```

## Running with Docker

You can also use it within a Docker container. To do that, you'll need to
execute something more-or-less like the following:

```bash
docker run --rm -v $PWD:/tmp -w /tmp goreleaser/nfpm package \
	--config /tmp/pkg/foo.yml \
	--target /tmp \
	--packager deb
```

## Packaging status

[![Packaging status](https://repology.org/badge/vertical-allrepos/nfpm.svg)](https://repology.org/project/nfpm/versions)

## Compiling from source

Here you have two options:

If you want to contribute to the project, please follow the steps on our
[contributing guide](/contributing).

If you just want to build from source for whatever reason, follow these steps:

**clone:**

```bash
git clone https://github.com/goreleaser/nfpm
cd nfpm
```

**get the dependencies:**

```bash
go mod tidy
```

**build:**

```bash
go build -o nfpm ./cmd/nfpm
```

**verify it works:**

```bash
./nfpm --version
```

[releases]: https://github.com/goreleaser/nfpm/releases
