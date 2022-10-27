FROM debian AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.deb


# ---- minimal test ----
FROM test_base AS min
RUN dpkg -i /tmp/foo.deb


# ---- symlink test ----
FROM min AS symlink
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"


# ---- simple test ----
FROM min AS simple
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN dpkg -r foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/local/bin/fake


# ---- no-glob test ----
FROM min AS no-glob
RUN test -d /usr/share/whatever/
RUN test -f /usr/share/whatever/file1
RUN test -f /usr/share/whatever/file2
RUN test -d /usr/share/whatever/folder2
RUN test -f /usr/share/whatever/folder2/file1
RUN test -f /usr/share/whatever/folder2/file2


# ---- complex test ----
FROM min AS complex
RUN apt-cache show /tmp/foo.deb | grep "Depends: bash"
RUN apt-cache show /tmp/foo.deb | grep "Suggests: zsh"
RUN apt-cache show /tmp/foo.deb | grep "Recommends: fish"
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -d /usr/share/whatever/
RUN test -d /usr/share/whatever/folder
RUN test -f /usr/share/whatever/folder/file1
RUN test -f /usr/share/whatever/folder/file2
RUN test -d /usr/share/whatever/folder/folder2
RUN test -f /usr/share/whatever/folder/folder2/file1
RUN test -f /usr/share/whatever/folder/folder2/file2
RUN test -d /var/log/whatever
RUN test -d /usr/share/foo
RUN test -d /usr/foo/bar/something
RUN test -d /etc/something
RUN test -f /etc/something/a
RUN test -f /etc/something/b
RUN test -d /etc/something/c
RUN test -f /etc/something/c/d
RUN test $(stat -c %a /usr/sbin/fake) -eq 4755
RUN test -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN dpkg -r foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/local/bin/fake
RUN test ! -f /usr/sbin/fake
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof
RUN test ! -d /var/log/whatever
RUN test ! -d /usr/share/foo
RUN test ! -d /usr/foo/bar/something

# ---- signed test ----
FROM test_base AS signed
COPY keys/pubkey.gpg /usr/share/debsig/keyrings/9890904DFB2EC88A/debsig.gpg
RUN apt update -y
RUN apt install -y debsig-verify
COPY deb.policy.pol /etc/debsig/policies/9890904DFB2EC88A/policy.pol
# manually check signature
RUN debsig-verify /tmp/foo.deb | grep "debsig: Verified package from 'Test package' (test)"
# clear dpkg config as it contains 'no-debsig', now every
# package that will be installed must be signed
RUN echo "" > /etc/dpkg/dpkg.cfg
RUN dpkg -i /tmp/foo.deb

# ---- signed dpkg-sig test ----
FROM test_base AS dpkg-signed
RUN apt update -y
RUN apt install -y dpkg-sig
# TODO: we should properly check the signature here, not sure how to do so.
RUN dpkg-sig --verify /tmp/foo.deb | grep "UNKNOWNSIG _gpgbuilder 15BD80B3"
RUN dpkg -i /tmp/foo.deb

# ---- overrides test ----
FROM min AS overrides
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN dpkg -r foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/local/bin/fake
RUN test -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof


# ---- meta test ----
FROM test_base AS meta
RUN apt update && apt install -y /tmp/foo.deb
RUN command -v zsh


# ---- env-var-version test ----
FROM min AS env-var-version
ENV EXPECTVER=" Version: 1.0.0~0.1.b1+git.abcdefgh"
RUN dpkg --info /tmp/foo.deb | grep "Version" > found
RUN export FOUND_VER="$(cat found)" && \
	echo "Expected: '${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
	test "${FOUND_VER}" = "${EXPECTVER}"


# ---- changelog test ----
FROM test_base AS withchangelog
# the dpkg configuration of the docker
# image filters out changelogs by default
# so we have to remove that rule
RUN apt update -y
RUN apt install -y gzip
RUN dpkg -i /tmp/foo.deb
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 1"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 2"
RUN zcat "/usr/share/doc/foo/changelog.gz" | grep "note 3"

# ---- rules test ----
FROM min AS rules
RUN dpkg -r foo

# ---- triggers test ----
FROM min as triggers
# simulate another package that activates the trigger
RUN dpkg-trigger --by-package foo manual-trigger
RUN dpkg --triggers-only foo
RUN test -f /tmp/trigger-proof

# ---- breaks test ----
FROM test_base AS breaks
COPY dummy.deb /tmp/dummy.deb
# install dummy package
RUN dpkg -i /tmp/dummy.deb
# make sure foo can't be installed
RUN dpkg -i /tmp/foo.deb 2>&1 | grep "foo breaks dummy"
# make sure foo can be installed if dummy is not installed
RUN dpkg -r dummy
RUN dpkg -i /tmp/foo.deb

# ---- compression test ----
FROM min AS compression
RUN test -e /usr/local/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN dpkg -r foo
RUN test ! -f /usr/local/bin/fake

# ---- upgrade test ----
FROM test_base AS upgrade
ARG oldpackage
RUN echo "${oldpackage}"
COPY ${oldpackage} /tmp/old_foo.deb
RUN dpkg -i /tmp/old_foo.deb

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Install"

RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Install"

RUN dpkg -i /tmp/foo.deb

RUN test -f /tmp/preremove-proof
RUN cat /tmp/preremove-proof | grep "Upgrade"

RUN test -f /tmp/postremove-proof
RUN cat /tmp/postremove-proof | grep "Upgrade"

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Upgrade"

RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Upgrade"
