# Configuration

## Reference

A commented out `nfpm.yaml` config file example:

```yaml
# Name. (required)
name: foo

# Architecture. (required)
# This will expand any env var you set in the field, eg version: ${GOARCH}
arch: amd64

# Platform.
# Defaults to `linux`.
platform: linux

# Version. (required)
# This will expand any env var you set in the field, eg version: v${SEMVER}
version: v1.2.3

# Version Schema allows you to specify how to parse the version String.
# Default is `semver`
#   `semver` parse the version string as a valid semver version
#   `none` skip trying to parse the version string and just use what is passed in
version_schema: semver

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
# This will expand any env var you set in the field, eg release: ${VERSION_RELEASE}
release: 1

# Section.
section: default

# Priority.
# Defaults to `optional` on deb
# Defaults to empty on rpm and apk
priority: extra

# Maintainer.
# Defaults to empty on rpm and apk
# Leaving the 'maintainer' field unset will not be allowed in a future version
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

  # Simple symlink at /usr/local/bin/foo which points to /sbin/foo, which is
  # the same behaviour as `ln -s /sbin/foo /usr/local/bin/foo`.
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

  # Corresponds to `%config(noreplace)` if the packager is rpm, otherwise it is just a config file
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
  # that differs from what is on the local build system at build time.
  - src: path/to/foo
    dst: /usr/local/foo
    file_info:
      mode: 0644
      mtime: 2008-01-02T15:04:05Z
      owner: notRoot
      group: notRoot

  # Using the type 'dir', empty directories can be created. When building RPMs, however, this
  # type has another important purpose: Claiming ownership of that folder. This is important
  # because when upgrading or removing an RPM package, only the directories for which it has
  # claimed ownership are removed. However, you should not claim ownership of a folder that
  # is created by the distro or a dependency of your package.
  - dst: /some/dir
    type: dir
    file_info:
      mode: 0700

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
  summary: Explicit Summary for Sample Package

  # Compression algorithm (gzip (default), lzma or xz).
  compression: lzma

  # The package is signed if a key_file is set
  signature:
    # PGP secret key (can also be ASCII-armored), the passphrase is taken
    # from the environment variable $NFPM_RPM_PASSPHRASE with a fallback
    # to $NFPM_PASSPHRASE.
    # This will expand any env var you set in the field, eg key_file: ${SIGNING_KEY_FILE}
    key_file: key.gpg
    # PGP secret key id in hex format, if it is not set it will select the first subkey
    # that has the signing flag set. You may need to set this if you want to use the primary key as the signing key
    # or to support older versions of RPM < 4.13.0 which cannot validate a signed RPM that used a subkey to sign
    # This will expand any env var you set in the field, eg key_id: ${RPM_SIGNING_KEY_ID}
    key_id: bc8acdd415bd80b3

# Custom configuration applied only to the Deb packager.
deb:
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

  # Compression algorithm (gzip (default), xz or none).
  compression: xz

  # The package is signed if a key_file is set
  signature:
    # PGP secret key (can also be ASCII-armored). The passphrase is taken
    # from the environment variable $NFPM_DEB_PASSPHRASE with a fallback
    # to $NFPM_PASSPHRASE.
    # This will expand any env var you set in the field, eg key_file: ${SIGNING_KEY_FILE}
    key_file: key.gpg
    # The type describes the signers role, possible values are "origin",
    # "maint" and "archive". If unset, the type defaults to "origin".
    type: origin
    # PGP secret key id in hex format, if it is not set it will select the first subkey
    # that has the signing flag set. You may need to set this if you want to use the primary key as the signing key
    # This will expand any env var you set in the field, eg key_id: ${DEB_SIGNING_KEY_ID}
    key_id: bc8acdd415bd80b3

apk:
  # The package is signed if a key_file is set
  signature:
    # RSA private key in the PEM format. The passphrase is taken from
    # the environment variable $NFPM_APK_PASSPHRASE with a fallback
    # to $NFPM_PASSPHRASE.
    # This will expand any env var you set in the field, eg key_file: ${SIGNING_KEY_FILE}
    key_file: key.gpg
    # The name of the signing key. When verifying a package, the signature
    # is matched to the public key store in /etc/apk/keys/<key_name>.rsa.pub.
    # If unset, it defaults to the maintainer email address.
    key_name: origin
    # APK does not use pgp keys, so the key_id field is ignored.
    key_id: ignored
```

## Templating

Templating is not and will not be supported.

If you really need it, you can build on top of nFPM, use `envsubst`, `jsonnet`
or apply some other templating on top of it.

## JSON Schema

nFPM also has a [jsonschema][] file which you can use to have better editor support:

```
https://nfpm.goreleaser.com/schema.json
```

You can also generate it for your specific version using the [`nfpm jsonschema`][schema] command.

Note that it is in early stages.
Any help and/or feedback is greatly appreciated!

[jsonschema]: http://json-schema.org/draft/2020-12/json-schema-validation.html
[schema]: /cmd/nfpm_jsonschema/
