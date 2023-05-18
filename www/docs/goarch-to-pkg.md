# Go's GOARCH to packager

nFPM was branched out of [GoReleaser](https://goreleaser.com), so some if it
lean towards "the Go way" (whatever that means).

GoReleaser passes a string joining `GOARCH`, `GOARM`, etc as the package
architecture, and nFPM converts to the correct one for each packager.

Bellow is a list of the current conversions that are made.
Please, feel free to open an issue if you see anything wrong, or if you know the
correct value of some missing architecture.

Thank you!

---

## `deb`

| GOARCH | Value |
| :--: | :--: |
| `386` | `i386` |
| `amd64` | `x86_64` |
| `arm64` | `arm64` |
| `armv5` | `armel` |
| `armv6` | `armhf` |
| `armv7` | `armhf` |
| `mips64le` | `mips64el` |
| `mips` | `mips` |
| `mipsle` | `mipsel` |
| `ppc64le` | `ppc64el` |
| `s390` | `s390x` |

## `rpm`

| GOARCH | Value |
| :--: | :--: |
| `386` | `i386` |
| `amd64` | `x86_64` |
| `arm64` | `aarch64` |
| `armv5` | `armv5tel` |
| `armv6` | `armv6hl` |
| `armv7` | `armv7hl` |
| `mips64le` | `mips64el` |
| `mips` | `mips` |
| `mipsle` | `mipsel` |

## `apk`

| GOARCH | Value |
| :--: | :--: |
| `386` | `x86` |
| `amd64` | `x86_64` |
| `arm64` | `aarch64` |
| `armv6` | `armhf` |
| `armv7` | `armv7` |
| `ppc64le` | `ppc64le` |
| `s390` | `s390x` |

## `archlinux`

| GOARCH | Value |
| :--: | :--: |
| `386` | `i686` |
| `amd64` | `x86_64` |
| `arm64` | `aarch64` |
| `armv5` | `arm` |
| `armv6` | `arm6h` |
| `armv7` | `armv7h` |

