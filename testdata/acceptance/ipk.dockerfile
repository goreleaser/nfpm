FROM openwrt/rootfs AS test_base
ARG package
#ARG CACHEBUST=1 
RUN mkdir -p /var/lock && \
    mkdir -p /var/run && \
    mkdir -p /tmp
RUN echo "${package}"
COPY ${package} /tmp/foo.ipk

# ---- minimal test ----
FROM test_base AS min
RUN opkg install /tmp/foo.ipk


# ---- symlink test ----
FROM min AS symlink
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"


# ---- simple test ----
FROM min AS simple
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN opkg remove foo
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
FROM test_base AS complex
RUN opkg install coreutils-stat
RUN test "$(opkg status fish)" = ""
RUN opkg install /tmp/foo.ipk > install.log
RUN opkg depends foo | grep "bash"
RUN cat install.log | grep "package foo suggests installing zsh"
RUN test "$(opkg status fish)" != ""
RUN opkg info foo | grep "Provides: fake"
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
RUN opkg remove foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/bin/fake
RUN test ! -f /usr/bin/fake2
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof

# ---- overrides test ----
FROM min AS overrides
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN opkg remove foo
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /usr/bin/fake
RUN test -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof

# ---- meta test ----
FROM test_base AS meta
RUN opkg install /tmp/foo.ipk
RUN command -v zsh

# ---- env-var-version test ----
FROM min AS env-var-version
ENV EXPECTVER="Version: 1.0.0~0.1.b1+git.abcdefgh"
RUN opkg info foo | grep "Version" > found
RUN export FOUND_VER="$(cat found)" && \
	echo "Expected: '${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
	test "${FOUND_VER}" = "${EXPECTVER}"

# ---- changelog test ----
FROM test_base AS withchangelog

# ---- signed test ----
FROM test_base AS signed

# ----- IPK Specific Tests -----

# ---- alternatives test ----
FROM test_base AS alternatives
RUN test ! -e /usr/bin/foo
RUN test ! -e /usr/bin/bar
RUN test ! -e /usr/bin/baz
RUN opkg install /tmp/foo.ipk
RUN test -e /usr/bin/fake
RUN test -e /usr/bin/bar
RUN test -e /usr/bin/baz

# ---- conflicts test ----
FROM test_base AS conflicts
COPY dummy.ipk /tmp/dummy.ipk
# install dummy package
RUN opkg install /tmp/dummy.ipk
# make sure foo can't be installed
RUN opkg install /tmp/foo.ipk 2>&1 | grep "Cannot install package foo"
# make sure foo can be installed if dummy is not installed
RUN opkg remove dummy
RUN opkg install /tmp/foo.ipk

# ---- predepends test ----
FROM test_base AS predepends
COPY dummy.ipk /tmp/dummy.ipk
RUN opkg install /tmp/foo.ipk 2>&1 | grep "cannot find dependency dummy for foo"
RUN opkg install /tmp/dummy.ipk
RUN opkg install /tmp/foo.ipk


# ---- upgrade test ----
FROM test_base AS upgrade
ARG oldpackage
RUN echo "${oldpackage}"
COPY ${oldpackage} /tmp/old_foo.ipk
RUN opkg install /tmp/old_foo.ipk

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Install"

RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Install"

# The upgrade process doesn't allow a local upgrade.
RUN opkg install /tmp/foo.ipk

RUN test -f /tmp/preremove-proof
RUN cat /tmp/preremove-proof | grep "Upgrade"

RUN test -f /tmp/postremove-proof
RUN cat /tmp/postremove-proof | grep "Upgrade"

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Upgrade"

# The upgrade process doesn't allow a local upgrade,
# so the following test will fail.
#RUN test -d /tmp/postinstall-proof
#RUN cat /tmp/postinstall-proof | grep "Upgrade"
