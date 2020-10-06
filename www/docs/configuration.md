# Configuration

## Reference

A commented out `nfpm.yaml` config file example:

```yaml
# Name. (required)
name: foo

# Architecture. (required)
arch: amd64

# Platform.
# Defaults to `linux`.
platform: linux

# Version. (required)
version: v1.2.3

# Version Epoch.
# Default is extracted from `version` if it is semver compatible.
epoch: 2

# Version Prerelease.
# Default is extracted from `version` if it is semver compatible.
prerelease: beta1

# Version Metadata (previously deb.metadata).
# Default is extracted from `version` if it is semver compatible.
# Setting metadata might interfere with version comparisons depending on the packager.
version_metadata: git

# Version Release.
release: 1

# Section.
section: default

# Priority.
priority: extra

# Maintaner.
maintainer: Carlos Alexandro Becker <root@carlosbecker.com>

# Description.
# Defaults to `no description given`.
description: Sample package

# Vendor.
vendor: GoReleaser

# Package's homepage.
homepage: https://nfpm.goreleaser.com

# License.
license: MIT

# Changelog YAML file, see: https://github.com/goreleaser/chglog
changelog: "changelog.yaml"

# Packages it replaces. (overridable)
replaces:
  - foobar

# Packages it provides. (overridable)
provides:
  - bar

# Dependencies. (overridable)
depends:
  - git

# Recommended packages. (overridable)
recommends:
  - golang

# Suggested packages. (overridable)
suggests:
  - bzr

# Packages it conflicts with. (overridable)
conflicts:
  - mercurial

# Files to add to the package. (overridable)
# This can be binaries or any other files.
#
# Key is the local file/glob, value is the path inside the package.
files:
  path/to/local/foo: /usr/local/bin/foo
  some/glob/**/*: /usr/share/foo/

# Config files are dealt with differently when upgrading and uninstalling the
# package. (overridable)
#
# Key is the local file/glob, value is the path inside the package
config_files:
  path/to/local/foo.conf: /etc/foo.conf
  some/glob/**/*: /etc/foo/

# Empty folders your package may need created. (overridable)
empty_folders:
  - /var/log/foo

# Symlinks mapping from symlink name inside package to target inside package (overridable)
symlinks:
  /sbin/foo: /usr/local/bin/foo

# Scripts to run at specific stages. (overridable)
scripts:
  preinstall: ./scripts/preinstall.sh
  postinstall: ./scripts/postinstall.sh
  preremove: ./scripts/preremove.sh
  postremove: ./scripts/postremove.sh

# All fields above marked as `overridable` can be overriden for a given package format in this section.
overrides:
  # The depends override can for example be used to provide version constraints for dependencies where
  # different package formats use different versions or for dependencies that are named differently.
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

# Custon configuration applied only to the RPM packager.
rpm:
  # The package group. This option is deprecated by most distros
  # but required by old distros like CentOS 5 / EL 5 and earlier.
  group: Unspecified

  # Compression algorithm.
  compression: lzma

  # These config files will not be replaced by new versions if they were
  # changed by the user. Corresponds to %config(noreplace).
  config_noreplace_files:
    path/to/local/bar.con: /etc/bar.conf

  # The package is signed if a key_file is set
  signature:
    # PGP secret key (can also be ASCII-armored), the passphrase is taken
    # from the environment variable $NFPM_RPM_PASSPHRASE with a fallback
    # to #NFPM_PASSPHRASE.
    key_file: key.gpg

# Custon configuration applied only to the Deb packager.
deb:
  # Custom deb rules script.
  scripts:
    rules: foo.sh

  # Custom deb triggers
  triggers:
    # register interrest on a trigger activated by another package
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

  # The package is signed if a key_file is set
  signature:
    # PGP secret key (can also be ASCII-armored). The passphrase is taken
    # from the environment variable $NFPM_DEB_PASSPHRASE with a fallback
    # to #NFPM_PASSPHRASE.
    key_file: key.gpg
    # The type describes the signers role, possible values are "origin",
    # "maint" and "archive". If unset, the type defaults to "origin".
    type: origin

apk:
  # The package is signed if a key_file is set
  signature:
    # RSA private key in the PEM format. The passphrase is taken from
    # the environment variable $NFPM_APK_PASSPHRASE with a fallback
    # to #NFPM_PASSPHRASE.
    key_file: key.gpg
    # The name of the signing key. When verifying a package, the signature
    # is matched to the public key store in /etc/apk/keys/<key_name>.rsa.pub.
    # If unset, it defaults to the maintainer email address.
    key_name: origin
```

## Templating

Templating is not and will not be supported.

If you really need it, you can build on top of NFPM, use `envsubst`, `jsonnet`
or apply some other templating on top of it.
