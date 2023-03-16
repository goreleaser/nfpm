# Install

You can install the pre-compiled binary (in several ways), use Docker
or compile from source.

Bellow you can find the steps for each of them.

## Install the pre-compiled binary

### homebrew tap

```sh
brew install goreleaser/tap/nfpm
```

### homebrew

```sh
brew install nfpm
```

!!! info
    The [formula in homebrew-core](https://github.com/Homebrew/homebrew-core/blob/master/Formula/nfpm.rb) might be slightly outdated.
    Use our homebrew tap to always get the latest updates.

### scoop

```sh
scoop bucket add goreleaser https://github.com/goreleaser/scoop-bucket.git
scoop install nfpm
```

### apt

```sh
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
sudo apt update
sudo apt install nfpm
```

### yum

```sh
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

```sh
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
	```sh
	wget https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt
	wget https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt.sig
	wget https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt.pem
	```

1. Verify the signature:
	```sh
	cat checksums.txt.pem | base64 -d | openssl x509 -text -noout
	...
	            X509v3 Subject Alternative Name: critical
                URI:https://github.com/goreleaser/nfpm/.github/workflows/release.yml@refs/tags/v2.26.0
            1.3.6.1.4.1.57264.1.1:
                https://token.actions.githubusercontent.com

	...
	cosign verify-blob \
		--certificate checksum.txt.pem \
		--signature checksums.txt.sig \
		--certificate-identity https://github.com/goreleaser/nfpm/.github/workflows/release.yml@refs/tags/v2.26.0 \
		--certificate-oidc-issuer https://token.actions.githubusercontent.com \
		checksums.txt
	Verfied OK
	```
1. If the signature is valid, you can then verify the SHA256 sums match with the
   downloaded binary:
	```sh
	sha256sum --ignore-missing -c checksums.txt
	```

### docker images

Our Docker images are signed with [cosign][].

Verify the signature:

```sh
COSIGN_EXPERIMENTAL=1 cosign verify	goreleaser/nfpm
COSIGN_EXPERIMENTAL=1 cosign verify ghcr.io/goreleaser/nfpm
```

## Running with Docker

You can also use it within a Docker container. To do that, you'll need to
execute something more-or-less like the following:

```sh
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

```sh
git clone https://github.com/goreleaser/nfpm
cd nfpm
```

**get the dependencies:**

```sh
go mod tidy
```

**build:**

```sh
go build -o nfpm ./cmd/nfpm
```

**verify it works:**

```sh
./nfpm --version
```

[releases]: https://github.com/goreleaser/nfpm/releases
