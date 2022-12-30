FROM archlinux AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.pkg.tar.zst


# ---- minimal test ----
FROM test_base AS min
RUN pacman --noconfirm -U /tmp/foo.pkg.tar.zst


# ---- symlink test ----
FROM min AS symlink
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"


# ---- simple test ----
FROM min AS simple
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN pacman --noconfirm -R foo
RUN test -f /etc/foo/whatever.conf.pacsave
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
RUN pacman -Qi foo | grep "Depends On\\s*: bash"
RUN pacman -Qi foo | grep "Replaces\\s*: foo"
RUN pacman -Qi foo | grep "Provides\\s*: fake"
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
RUN pacman --noconfirm -R foo
RUN test -f /etc/foo/whatever.conf.pacsave
RUN test ! -f /usr/bin/fake
RUN test ! -f /usr/bin/fake2
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof
RUN test ! -d /var/log/whatever
RUN test ! -d /usr/share/foo
RUN test ! -d /usr/foo/bar/something


# ---- signed test ----
FROM min AS signed
RUN echo "Arch Linux has no signature support"


# ---- overrides test ----
FROM min AS overrides
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test ! -f /tmp/preinstall-proof
RUN test -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN pacman --noconfirm -R foo
RUN test -f /etc/foo/whatever.conf.pacsave
RUN test ! -f /usr/bin/fake
RUN test -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof


# ---- meta test ----
FROM test_base AS meta
RUN pacman -Sy && pacman --noconfirm -U /tmp/foo.pkg.tar.zst
RUN command -v zsh


# ---- env-var-version test ----
FROM min AS env-var-version
ENV EXPECTVER="foo 1.0.0-1"
RUN export FOUND_VER="$(pacman -Q foo)" && \
	echo "Expected: '${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
	test "${FOUND_VER}" = "${EXPECTVER}"


# ---- changelog test ----
FROM min AS withchangelog
RUN echo "Arch Linux has no changelog support"


# ---- upgrade test ----
FROM test_base AS upgrade
ARG oldpackage
RUN echo "${oldpackage}"
COPY ${oldpackage} /tmp/old_foo.pkg.tar.zst
RUN pacman --noconfirm -U /tmp/old_foo.pkg.tar.zst
RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Install"
RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Install"
RUN test ! -f /tmp/preupgrade-proof
RUN test ! -f /tmp/postupgrade-proof
RUN echo modified > /etc/regular.conf
RUN echo modified > /etc/noreplace.conf
RUN pacman --noconfirm -U /tmp/foo.pkg.tar.zst
RUN test -f /tmp/preupgrade-proof
RUN test -f /tmp/postupgrade-proof
RUN test -f /etc/regular.conf
RUN test -f /etc/regular.conf.pacnew
RUN test -f /etc/noreplace.conf
RUN test -f /etc/noreplace.conf.pacnew
