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

# Disables globbing for files, config_files, etc.
disable_globbing: false

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
    
# Contents to add to the package
# This can be binaries or any other files.
contents:
    # Basic file that applies to all packagers
  - src: path/to/local/foo
    dst: /usr/local/bin/foo
    
    # Simple config file
  - src: path/to/local/foo.conf
    dst: /etc/foo.conf
    type: config
    
    # Simple symlink
  - src: /sbin/foo # link name
    dst: /usr/local/bin/foo # real location
    type: "symlink"
    
    # Corresponds to %config(noreplace) if the packager is rpm, otherwise it is just a config file
  - src: path/to/local/bar.conf
    dst: /etc/bar.conf
    type: "config|noreplace"

    # These files are not actually present in the package, but the file names
    # are added to the package header. From the RPM directives documentation:
    #
    # "There are times when a file should be owned by the package but not
    # installed - log files and state files are good examples of cases you might
    # desire this to happen."
    #
    # "The way to achieve this, is to use the %ghost directive. By adding this
    # directive to the line containing a file, RPM will know about the ghosted
    # file, but will not add it to the package."
    # 
    # For non rpm packages ghost files are ignored at this time.
  - dst: /etc/casper.conf
    type: ghost
  - dst: /var/log/boo.log
    type: ghost
    
    # You can user the packager field to add files that are unique to a specific packager 
  - src: path/to/rpm/file.conf
    dst: /etc/file.conf
    type: "config|noreplace"
    packager: rpm
  - src: path/to/deb/file.conf
    dst: /etc/file.conf
    type: "config|noreplace"
    packager: deb
  - src: path/to/apk/file.conf
    dst: /etc/file.conf
    type: "config|noreplace"
    packager: apk
    
    # Sometimes it is important to be able to set the mtime, mode, owner, or group for a file 
    # that differs from what is on the local build system at build time.
  - src: path/to/foo
    dst: /usr/local/foo
    file_info:
      mode: 0644
      mtime: 2008-01-02T15:04:05Z
      owner: notRoot
      group: notRoot


# Empty folders your package may need created. (overridable)
empty_folders:
  - /var/log/foo

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

# Custom configuration applied only to the RPM packager.
rpm:
  # The package group. This option is deprecated by most distros
  # but required by old distros like CentOS 5 / EL 5 and earlier.
  group: Unspecified

  # The package summary. This is, by default, the first line of the
  # description, but can be explicitly provided here.
  summary: Explicit Summary for Sample Package

  # Compression algorithm.
  compression: lzma

  # The package is signed if a key_file is set
  signature:
    # PGP secret key (can also be ASCII-armored), the passphrase is taken
    # from the environment variable $NFPM_RPM_PASSPHRASE with a fallback
    # to #NFPM_PASSPHRASE.
    key_file: key.gpg

# Custom configuration applied only to the Deb packager.
deb:
  # Custom deb special files.
  scripts:
    # Deb rules script.
    rules: foo.sh
    # Deb templates file, when using debconf.
    templates: templates

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
