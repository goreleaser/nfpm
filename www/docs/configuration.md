# Configuration

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
  # Group.
  group: root

  # Compression algorithm.
  compression: lzma

# Custon configuration applied only to the Deb packager.
# All fields described bellow, plus all fields above marked as `overridable`
# can be specified here.
deb:
  # Custom version metadata.
  # Default is extracted from `version` if it is semver compatible.
  metadata: xyz2

  # Custom deb rules script.
  scripts:
    rules: foo.sh
```

## Templating

Templating is not and will not be supported.

If you really need it, you can build on top of NFPM, use `envsubst`, `jsonnet`
or something apply it on top of it.
