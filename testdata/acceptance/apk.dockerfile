FROM alpine:3.23.3 AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.apk


# ---- minimal test ----
FROM test_base AS min
RUN apk add --allow-untrusted /tmp/foo.apk


# ---- symlink test ----
FROM min AS symlink
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"


# ---- simple test ----
FROM min AS simple
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN apk del foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/bin/fake


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
RUN test -e /usr/bin/fake
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
RUN test $(stat -c %a /usr/bin/fake2) -eq 4755
RUN test -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN apk del foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/bin/fake
RUN test ! -f /usr/bin/fake2
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof
RUN test ! -d /var/log/whatever
RUN test ! -d /usr/share/foo
RUN test ! -d /usr/foo/bar/something


# ---- signed test ----
FROM test_base AS signed
COPY keys/rsa_unprotected.pub /etc/apk/keys/john@example.com.rsa.pub
RUN apk verify /tmp/foo.apk | grep "/tmp/foo.apk: OK"
RUN apk add  /tmp/foo.apk


# ---- overrides test ----
FROM min AS overrides
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN apk del foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/bin/fake
RUN test -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof


# ---- meta test ----
FROM min AS meta
RUN command -v zsh


# ---- env-var-version test ----
FROM min AS env-var-version
ENV EXPECTVER="foo-1.0.0_0.1.b1-git.abcdefgh description:"
RUN apk info foo | grep "foo-" | grep " description:" > found
RUN export FOUND_VER="$(cat found)" && \
	echo "Expected: '${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
	test "${FOUND_VER}" = "${EXPECTVER}"


# ---- changelog test ----
FROM min AS withchangelog
RUN echo "No Changelog support for apk?"


# ---- upgrade test ----
FROM test_base AS upgrade
ARG oldpackage
RUN echo "${oldpackage}"
COPY ${oldpackage} /tmp/old_foo.apk
RUN apk add --allow-untrusted /tmp/old_foo.apk

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Install"

RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Install"

RUN test ! -f /tmp/preupgrade-proof
RUN test ! -f /tmp/postupgrade-proof

RUN echo modified > /etc/regular.conf
RUN echo modified > /etc/noreplace.conf

RUN apk add --allow-untrusted /tmp/foo.apk

RUN test -f /tmp/preupgrade-proof
RUN test -f /tmp/postupgrade-proof
