# Configuration

## Reference

A commented `nfpm.yaml` config file example:

```yaml
# Name. (required)
name: foo

# Architecture. (required)
# This will expand any env var you set in the field, e.g. version: ${GOARCH}
# The architecture is specified using Go nomenclature (GOARCH) and translated
# to the platform specific equivalent. In order to manually set the architecture
# to a platform specific value, use deb_arch, rpm_arch and apk_arch.
# Examples: `all`, `amd64`, `386`, `arm5`, `arm6`, `arm7`, `arm64`, `mips`,
# `mipsle`, `mips64le`, `ppc64le`, `s390`
arch: amd64

# Platform.
# This will expand any env var you set in the field, e.g. version: ${GOOS}
# This is only used by the rpm and deb packagers.
# Examples: `linux` (default), `darwin`
platform: linux

# Version. (required)
# This will expand any env var you set in the field, e.g. version: ${SEMVER}
# Some package managers, like deb, require the version to start with a digit.
# Hence, you should not prefix the version with 'v'.
version: 1.2.3

# Version Schema allows you to specify how to parse the version string.
# Default is `semver`
#   `semver` attempt to parse the version string as a valid semver version.
#       The parser is lenient; it will strip a `v` prefix and will accept
#       versions with fewer than 3 components, like `v1.2`.
#       If parsing succeeds, then the version will be molded into a format
#       compatible with the specific packager used.
#       If parsing fails, then the version is used as-is.
#   `none` skip trying to parse the version string and just use what is passed in
version_schema: semver

# Version Epoch.
# A package with a higher version epoch will always be considered newer.
# See: https://www.debian.org/doc/debian-policy/ch-controlfields.html#epochs-should-be-used-sparingly
epoch: 2

# Version Prerelease.
# Default is extracted from `version` if it is semver compatible.
# This is appended to the `version`, e.g. `1.2.3+beta1`. If the `version` is
# semver compatible, then this replaces the prerelease component of the semver.
prerelease: beta1

# Version Metadata (previously deb.metadata).
# Default is extracted from `version` if it is semver compatible.
# Setting metadata might interfere with version comparisons depending on the
# packager. If the `version` is semver compatible, then this replaces the
# version metadata component of the semver.
version_metadata: git

# Version Release, aka revision.
# This will expand any env var you set in the field, e.g. release: ${VERSION_RELEASE}
# This is appended to the `version` after `prerelease`. This should be
# incremented if you release an updated package of the same upstream version,
# and it should reset to 1 when bumping the version.
release: 1

# Section.
# This is only used by the deb packager.
# See: https://www.debian.org/doc/debian-policy/ch-archive.html#sections
section: default

# Priority.
# Defaults to `optional` on deb
# Defaults to empty on rpm and apk
# See: https://www.debian.org/doc/debian-policy/ch-archive.html#priorities
priority: extra

# Maintainer. (required)
# This will expand any env var you set in the field, e.g. maintainer: ${GIT_COMMITTER_NAME} <${GIT_COMMITTER_EMAIL}>
# Defaults to empty on rpm and apk
# Leaving the 'maintainer' field unset will not be allowed in a future version
maintainer: Carlos Alexandro Becker <root@carlosbecker.com>

# Description.
# Defaults to `no description given`.
# Most packagers call for a one-line synopsis of the package. Some (like deb)
# also call for a multi-line description starting on the second line.
description: Sample package

# Vendor.
# This will expand any env var you set in the field, e.g. vendor: ${VENDOR}
# This is only used by the rpm packager.
vendor: GoReleaser

# Package's homepage.
homepage: https://nfpm.goreleaser.com

# License.
license: MIT

# Changelog YAML file, see: https://github.com/goreleaser/chglog
changelog: "changelog.yaml"

# Disables globbing for files, config_files, etc.
disable_globbing: false

# Packages it replaces. (overridable)
# This will expand any env var you set in the field, e.g. ${REPLACE_BLA}
# the env var approach can be used to account for differences in platforms
replaces:
  - foobar
  - ${REPLACE_BLA}

# Packages it provides. (overridable)
# This will expand any env var you set in the field, e.g. ${PROVIDES_BLA}
# the env var approach can be used to account for differences in platforms
provides:
  - bar
  - ${PROVIDES_BLA}

# Dependencies. (overridable)
# This will expand any env var you set in the field, e.g. ${DEPENDS_NGINX}
# the env var approach can be used to account for differences in platforms
# e.g. rhel needs nginx >= 1:1.18 and deb needs nginx (>= 1.18.0)
depends:
  - git
  - ${DEPENDS_NGINX}

# Recommended packages. (overridable)
# This will expand any env var you set in the field, e.g. ${RECOMMENDS_BLA}
# the env var approach can be used to account for differences in platforms
recommends:
  - golang
  - ${RECOMMENDS_BLA}

# Suggested packages. (overridable)
# This will expand any env var you set in the field, e.g. ${SUGGESTS_BLA}
# the env var approach can be used to account for differences in platforms
suggests:
  - bzr

# Packages it conflicts with. (overridable)
# This will expand any env var you set in the field, e.g. ${CONFLICTS_BLA}
# the env var approach can be used to account for differences in platforms
conflicts:
  - mercurial
  - ${CONFLICTS_BLA}

# Contents to add to the package
# This can be binaries or any other files.
contents:
  # Basic file that applies to all packagers
  - src: path/to/local/foo
    dst: /usr/bin/foo

  # This will add all files in some/directory or in subdirectories at the
  # same level under the directory /etc. This means the tree structure in
  # some/directory will not be replicated.
  - src: some/directory/
    dst: /etc

  # This will replicate the directory structure under some/directory at /etc.
  - src: some/directory/
    dst: /etc
    type: tree

  # Simple config file
  - src: path/to/local/foo.conf
    dst: /etc/foo.conf
    type: config

  # Select files with a glob (doesn't work if you set disable_globbing: true).
  # If `src` is a glob, then the `dst` will be treated like a directory - even
  # if it doesn't end with `/`, and even if the glob only matches one file.
  - src: path/to/local/*.1.gz
    dst: /usr/share/man/man1/

  # Simple symlink at /usr/bin/foo which points to /sbin/foo, which is
  # the same behaviour as `ln -s /sbin/foo /usr/bin/foo`.
  #
  # This also means that both "src" and "dst" are paths inside the package (or
  # rather paths in the file system where the package will be installed) and
  # not in the build environment. This is different from regular files where
  # "src" is a path in the build environment. However, this convention results
  # in "dst" always being the file that is created when installing the
  # package.
  - src: /actual/path/to/foo
    dst: /usr/bin/foo
    type: symlink

  # Corresponds to `%config(noreplace)` if the packager is rpm, otherwise it
  # is just a config file
  - src: path/to/local/bar.conf
    dst: /etc/bar.conf
    type: config|noreplace

  # These files are not actually present in the package, but the file names
  # are added to the package header. From the RPM directives documentation:
  #
  # "There are times when a file should be owned by the package but not
  # installed - log files and state files are good examples of cases you might
  # desire this to happen."
  #
  # "The way to achieve this is to use the %ghost directive. By adding this
  # directive to the line containing a file, RPM will know about the ghosted
  # file, but will not add it to the package."
  #
  # For non rpm packages ghost files are ignored at this time.
  - dst: /etc/casper.conf
    type: ghost
  - dst: /var/log/boo.log
    type: ghost

  # You can use the packager field to add files that are unique to a specific
  # packager
  - src: path/to/rpm/file.conf
    dst: /etc/file.conf
    type: config|noreplace
    packager: rpm
  - src: path/to/deb/file.conf
    dst: /etc/file.conf
    type: config|noreplace
    packager: deb
  - src: path/to/apk/file.conf
    dst: /etc/file.conf
    type: config|noreplace
    packager: apk

  # Sometimes it is important to be able to set the mtime, mode, owner, or group for a file
  # that differs from what is on the local build system at build time. The owner (if different
  # than 'root') has to be always specified manually in 'file_info' as it will not be copied
  # from the 'src' file.
  - src: path/to/foo
    dst: /usr/share/foo
    file_info:
      # Make sure that the mode is specified in octal, e.g. 0644 instead of 644.
      mode: 0644
      mtime: 2008-01-02T15:04:05Z
      owner: notRoot
      group: notRoot

  # Using the type 'dir', empty directories can be created. When building RPMs, however, this
  # type has another important purpose: Claiming ownership of that folder. This is important
  # because when upgrading or removing an RPM package, only the directories for which it has
  # claimed ownership are removed. However, you should not claim ownership of a folder that
  # is created by the distro or a dependency of your package.
  # A directory in the build environment can optionally be provided in the 'src' field in
  # order copy mtime and mode from that directory without having to specify it manually.
  - dst: /some/dir
    type: dir
    file_info:
      mode: 0700

# Scripts to run at specific stages. (overridable)
scripts:
  preinstall: ./scripts/preinstall.sh
  postinstall: ./scripts/postinstall.sh
  preremove: ./scripts/preremove.sh
  postremove: ./scripts/postremove.sh

# All fields above marked as `overridable` can be overridden for a given
# package format in this section.
overrides:
  # The depends override can for example be used to provide version
  # constraints for dependencies where different package formats use different
  # versions or for dependencies that are named differently.
  deb:
    depends:
      - baz (>= 1.2.3-0)
      - some-lib-dev
    # ...
  rpm:
    depends:
      - baz >= 1.2.3-0
      - some-lib-devel
    # ...
  apk:
    # ...
  archlinux:
    depends:
      - baz
      - some-lib

# Custom configuration applied only to the RPM packager.
rpm:
  # rpm specific architecture name that overrides "arch" without performing any
  # replacements.
  rpm_arch: ia64

  # RPM specific scripts.
  scripts:
    # The pretrans script runs before all RPM package transactions / stages.
    pretrans: ./scripts/pretrans.sh
    # The posttrans script runs after all RPM package transactions / stages.
    posttrans: ./scripts/posttrans.sh

  # The package group. This option is deprecated by most distros
  # but required by old distros like CentOS 5 / EL 5 and earlier.
  group: Unspecified

  # The package summary. This is, by default, the first line of the
  # description, but can be explicitly provided here.
  summary: Explicit summary for the package

  # The packager is used to identify the organization that actually packaged
  # the software, as opposed to the author of the software.
  # `maintainer` will be used as fallback if not specified.
  # This will expand any env var you set in the field, e.g. packager: ${PACKAGER}
  packager: GoReleaser <staff@goreleaser.com>

  # Compression algorithm (gzip (default), zstd, lzma or xz).
  compression: zstd

  # The package is signed if a key_file is set
  signature:
    # PGP secret key (can also be ASCII-armored), the passphrase is taken
    # from the environment variable $NFPM_RPM_PASSPHRASE with a fallback
    # to $NFPM_PASSPHRASE.
    # This will expand any env var you set in the field, e.g. key_file: ${SIGNING_KEY_FILE}
    key_file: key.gpg

    # PGP secret key id in hex format, if it is not set it will select the first subkey
    # that has the signing flag set. You may need to set this if you want to use the primary key as the signing key
    # or to support older versions of RPM < 4.13.0 which cannot validate a signed RPM that used a subkey to sign
    # This will expand any env var you set in the field, e.g. key_id: ${RPM_SIGNING_KEY_ID}
    key_id: bc8acdd415bd80b3

# Custom configuration applied only to the Deb packager.
deb:
  # deb specific architecture name that overrides "arch" without performing any replacements.
  deb_arch: arm

  # Custom deb special files.
  scripts:
    # Deb rules script.
    rules: foo.sh

    # Deb templates file, when using debconf.
    templates: templates

    # Deb config maintainer script for asking questions when using debconf.
    config: config

  # Custom deb triggers
  triggers:
    # register interest on a trigger activated by another package
    # (also available: interest_await, interest_noawait)
    interest:
      - some-trigger-name

    # activate a trigger for another package
    # (also available: activate_await, activate_noawait)
    activate:
      - another-trigger-name

  # Packages which would break if this package would be installed.
  # The installation of this package is blocked if `some-package`
  # is already installed.
  breaks:
    - some-package

  # Compression algorithm (gzip (default), zstd, xz or none).
  compression: zstd

  # The package is signed if a key_file is set
  signature:
    # Signature method, either "dpkg-sig" or "debsign".
    # Defaults to "debsign"
    method: dpkg-sig

    # PGP secret key (can also be ASCII-armored). The passphrase is taken
    # from the environment variable $NFPM_DEB_PASSPHRASE with a fallback
    # to $NFPM_PASSPHRASE.
    # This will expand any env var you set in the field, e.g. key_file: ${SIGNING_KEY_FILE}
    key_file: key.gpg

    # The type describes the signers role, possible values are "origin",
    # "maint" and "archive". If unset, the type defaults to "origin".
    type: origin

    # PGP secret key id in hex format, if it is not set it will select the first subkey
    # that has the signing flag set. You may need to set this if you want to use the primary key as the signing key
    # This will expand any env var you set in the field, e.g. key_id: ${DEB_SIGNING_KEY_ID}
    key_id: bc8acdd415bd80b3

  # Additional fields for the control file. Empty fields are ignored.
  fields:
    Bugs: https://github.com/goreleaser/nfpm/issues

apk:
  # apk specific architecture name that overrides "arch" without performing any replacements.
  apk_arch: armhf

  # The package is signed if a key_file is set
  signature:
    # RSA private key in the PEM format. The passphrase is taken from
    # the environment variable $NFPM_APK_PASSPHRASE with a fallback
    # to $NFPM_PASSPHRASE.
    # This will expand any env var you set in the field, e.g. key_file: ${SIGNING_KEY_FILE}
    key_file: key.gpg

    # The name of the signing key. When verifying a package, the signature
    # is matched to the public key store in /etc/apk/keys/<key_name>.rsa.pub.
    # If unset, it defaults to the maintainer email address.
    key_name: origin

    # APK does not use pgp keys, so the key_id field is ignored.
    key_id: ignored

archlinux:
  # This value is used to specify the name used to refer to a group
  # of packages when building a split package. Defaults to name
  # See: https://wiki.archlinux.org/title/PKGBUILD#pkgbase
  pkgbase: bar

  # The packager identifies the organization packaging the software
  # rather than the developer. Defaults to "Unknown Packager".
  packager: GoReleaser <staff@goreleaser.com>

  # Arch Linux specific scripts.
  scripts:
    # The preupgrade script runs before pacman upgrades the package
    preupgrade: ./scripts/preupgrade.sh

    # The postupgrade script runs after pacman upgrades the package
    postupgrade: ./scripts/postupgrade.sh
```

## Templating

Templating is not and will not be supported.

If you really need it, you can build on top of nFPM, use `envsubst`, `jsonnet`
or apply some other templating on top of it.

## JSON Schema

nFPM also has a [jsonschema][] file which you can use to have better editor
support:

```
https://nfpm.goreleaser.com/schema.json
```

You can also generate it for your specific version using the
[`nfpm jsonschema`][schema] command.

Note that it is in early stages.
Any help and/or feedback is greatly appreciated!

[jsonschema]: http://json-schema.org/draft/2020-12/json-schema-validation.html
[schema]: /cmd/nfpm_jsonschema/
