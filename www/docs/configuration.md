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

# Version Release.
# Default is extracted from `version` if it is semver compatible.
release: 1

# Version Prerelease.
# Default is extracted from `version` if it is semver compatible.
prerelease: beta1

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
# Key is the local file, value is the path inside the package.
files:
  path/to/local/foo: /usr/local/bin/foo

# Config files are dealt with differently when upgrading and uninstalling the
# package. (overridable)
#
# Key is the local file, value is the path inside the package
config_files:
  path/to/local/foo.con: /etc/foo.conf

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

# Custon configuration applied only to the RPM packager.
# All fields described bellow, plus all fields above marked as `overridable`
# can be specified here.
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

# Custon configuration applied only to the Deb packager.
# All fields described bellow, plus all fields above marked as `overridable`
# can be specified here.
deb:
  # Custom version metadata.
  # Setting metadata might interfere with version comparisons.
  metadata: xyz2

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
```

## Templating

Templating is not and will not be supported.

If you really need it, you can build on top of NFPM, use `envsubst`, `jsonnet`
or apply some other templating on top of it.
