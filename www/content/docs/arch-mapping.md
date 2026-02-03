---
title: Architecture Mapping
weight: 5
---

nFPM was branched out of [GoReleaser](https://goreleaser.com), so some of it
lean towards "the Go way" (whatever that means).

GoReleaser passes a string joining `GOARCH`, `GOARM`, etc as the package
architecture, and nFPM converts to the correct one for each packager.

nFPM also accepts common architecture names from `uname -m` (like `x86_64` and
`aarch64`) and translates them to the correct value for each packager.

Below is a list of the current conversions that are made.
Please, feel free to open an issue if you see anything wrong, or if you know the
correct value of some missing architecture.

Thank you!

---

{{< tabs items="Deb,RPM,APK,Arch Linux" >}}

{{< tab >}}

|   Input    |   Value    |
| :--------: | :--------: |
|   `386`    |   `i386`   |
|  `amd64`   |  `amd64`   |
|  `arm64`   |  `arm64`   |
|   `arm5`   |  `armel`   |
|   `arm6`   |  `armhf`   |
|   `arm7`   |  `armhf`   |
| `mips64le` | `mips64el` |
|   `mips`   |   `mips`   |
|  `mipsle`  |  `mipsel`  |
| `ppc64le`  | `ppc64el`  |
|   `s390`   |  `s390x`   |
|  `x86_64`  |  `amd64`   |
| `aarch64`  |  `arm64`   |

{{< /tab >}}

{{< tab >}}

|   Input    |     Value     |
| :--------: | :-----------: |
|   `386`    |    `i386`     |
|  `amd64`   |   `x86_64`    |
|  `arm64`   |   `aarch64`   |
|   `arm5`   |  `armv5tel`   |
|   `arm6`   |   `armv6hl`   |
|   `arm7`   |   `armv7hl`   |
| `mips64le` |  `mips64el`   |
|   `mips`   |    `mips`     |
|  `mipsle`  |   `mipsel`    |
| `loong64`  | `loongarch64` |

{{< /tab >}}

{{< tab >}}

|   Input   |     Value     |
| :-------: | :-----------: |
|   `386`   |     `x86`     |
|  `amd64`  |   `x86_64`    |
|  `arm64`  |   `aarch64`   |
|  `arm6`   |    `armhf`    |
|  `arm7`   |    `armv7`    |
| `ppc64le` |   `ppc64le`   |
|  `s390`   |    `s390x`    |
| `loong64` | `loongarch64` |
| `x86_64`  |   `x86_64`    |
| `aarch64` |   `aarch64`   |
|  `i386`   |     `x86`     |
|  `i686`   |     `x86`     |

{{< /tab >}}

{{< tab >}}

|   Input   |   Value   |
| :-------: | :-------: |
|   `386`   |  `i686`   |
|  `amd64`  | `x86_64`  |
|  `arm64`  | `aarch64` |
|  `arm5`   |   `arm`   |
|  `arm6`   | `armv6h`  |
|  `arm7`   | `armv7h`  |
| `x86_64`  | `x86_64`  |
| `aarch64` | `aarch64` |
|  `i386`   |  `i686`   |

{{< /tab >}}

{{< /tabs >}}
