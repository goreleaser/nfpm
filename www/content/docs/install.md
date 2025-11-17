---
title: Install
weight: 2
---

You can install the pre-compiled binary (in several ways), use Docker or compile from source.

Below you can find the steps for each of them.

## Using a package manager

{{< tabs items="Homebrew Tap,Homebrew,Scoop,APT,Yum,Winget,NPM" >}}

{{< tab >}}

```bash
brew install goreleaser/tap/nfpm
```

{{< /tab >}}

{{< tab >}}

```bash
brew install nfpm
```

> [!INFO]
> The [formula in homebrew-core](https://github.com/Homebrew/homebrew-core/blob/master/Formula/n/nfpm.rb) might be slightly outdated.
> Use our homebrew tap to always get the latest updates.

{{< /tab >}}

{{< tab >}}

```bash
scoop bucket add goreleaser https://github.com/goreleaser/scoop-bucket.git
scoop install nfpm
```

{{< /tab >}}

{{< tab >}}

```bash
echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
sudo apt update
sudo apt install nfpm
```

{{< /tab >}}

{{< tab >}}

```bash
echo '[goreleaser]
name=GoReleaser
baseurl=https://repo.goreleaser.com/yum/
enabled=1
gpgcheck=0' | sudo tee /etc/yum.repos.d/goreleaser.repo
sudo yum install nfpm
```

{{< /tab >}}

{{< tab >}}

```bash
winget install --id=goreleaser.nfpm
```

{{< /tab >}}

{{< tab >}}

```bash
npm install -g @goreleaser/nfpm
# or
npx @goreleaser/nfpm
```

{{< /tab >}}

{{< /tabs >}}

## Pre-built packages and archives

Download the your format of choice from the
[releases](https://github.com/goreleaser/nfpm/releases)
and install them with the appropriate tools.

You may also download the archives and extract and run the binary inside.

## Running with Docker

You can also use it within a Docker container. To do that, you'll need to execute something more-or-less like the following:

```bash
docker run --rm -v $PWD:/tmp -w /tmp goreleaser/nfpm package \
	--config /tmp/pkg/foo.yml \
	--target /tmp \
	--packager deb
```

## Using go install

```bash
go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
```

## Verifying the artifacts

{{< tabs items="Binaries,Docker Images" >}}

{{< tab >}}

All artifacts are checksummed, and the checksum is signed with
[cosign](https://github.com/sigstore/cosign).

{{% steps %}}

### Download

Download the files you want, the `checksums.txt` and `checksums.txt.sig` files
from the [releases](https://github.com/goreleaser/nfpm/releases) page:

```bash
wget 'https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt'
```

### Verify the signature

```bash
wget 'https://github.com/goreleaser/nfpm/releases/download/__VERSION__/checksums.txt.sigstore.json'
cosign verify-blob \
  --certificate-identity 'https://github.com/goreleaser/nfpm/.github/workflows/release.yml@refs/tags/__VERSION__' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle "checksums.txt.sigstore.json" \
  checksums.txt
```

### Verify the checksums

If the signature is valid, you can then verify the SHA256 sums match with the downloaded binary:

```bash
sha256sum --ignore-missing -c checksums.txt
```

{{% /steps %}}

{{< /tab >}}

{{< tab >}}

Our Docker images are signed with [cosign](https://github.com/sigstore/cosign).

{{% steps %}}

### Pull the images

```bash
docker buill goreleaser/nfpm
# or
docker build ghcr.io/goreleaser/nfpm
```

### Verify

```bash
cosign verify goreleaser/nfpm
cosign verify ghcr.io/goreleaser/nfpm
```

{{% /steps %}}

{{< /tab >}}

{{< /tabs >}}

## Building from source

Here you have two options:

If you want to contribute to the project, please follow the steps on our [contributing guide](/docs/contributing).

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

## Packaging status

[![Packaging status](https://repology.org/badge/vertical-allrepos/nfpm.svg)](https://repology.org/project/nfpm/versions)
