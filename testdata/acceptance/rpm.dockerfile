FROM fedora AS test_base
ARG package
RUN echo "${package}"
COPY ${package} /tmp/foo.rpm


# ---- minimal test ----
FROM test_base AS min
RUN rpm -ivh /tmp/foo.rpm


# ---- symlink test ----
FROM min AS symlink
RUN ls -l /path/to/symlink | grep "/path/to/symlink -> /etc/foo/whatever.conf"


# ---- simple test ----
FROM min AS simple
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
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
RUN test "$(rpm -qp --recommends /tmp/foo.rpm)" = "fish"
RUN test "$(rpm -qp --suggests /tmp/foo.rpm)" = "zsh"
RUN test "$(rpm -qp --requires /tmp/foo.rpm)" = "bash"
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
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
RUN test -f /tmp/pretrans-proof
RUN test -f /tmp/posttrans-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/bin/fake
RUN test ! -f /usr/bin/fake2
RUN test -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof
RUN test ! -d /var/log/whatever
RUN test ! -d /usr/share/foo
RUN test ! -d /usr/foo/bar/something


# ---- signed test ----
FROM test_base AS signed
COPY keys/pubkey.asc /tmp/pubkey.asc
RUN rpm --import /tmp/pubkey.asc
RUN rpm -q gpg-pubkey --qf '%{NAME}-%{VERSION}-%{RELEASE}\t%{SUMMARY}\n'
RUN rpm -vK /tmp/foo.rpm
RUN rpm -vK /tmp/foo.rpm | grep "RSA/SHA256 Signature, key ID 15bd80b3: OK"
RUN rpm -K /tmp/foo.rpm
RUN rpm -K /tmp/foo.rpm | grep -E "(?:pgp|digests signatures) OK"

# Test with a repo
RUN yum install -y createrepo yum-utils
RUN rm -rf /etc/yum.repos.d/*.repo
COPY keys/test.rpm.repo /etc/yum.repos.d/test.rpm.repo
RUN createrepo /tmp
RUN yum install -y foo


# ---- overrides test ----
FROM min AS overrides
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN test -d /usr/share/whatever/folder
RUN test -f /usr/share/whatever/folder/file1
RUN test -f /usr/share/whatever/folder/file2
RUN test -d /usr/share/whatever/folder/folder2
RUN test -f /usr/share/whatever/folder/folder2/file1
RUN test -f /usr/share/whatever/folder/folder2/file2
RUN test -f /tmp/preinstall-proof
RUN test ! -f /tmp/postinstall-proof
RUN test ! -f /tmp/preremove-proof
RUN test ! -f /tmp/postremove-proof
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/bin/fake
RUN test ! -f /tmp/preremove-proof
RUN test -f /tmp/postremove-proof


# ---- meta test ----
FROM test_base AS meta
RUN dnf install -y /tmp/foo.rpm
RUN command -v zsh


# ---- env-var-version test ----
FROM min AS env-var-version
ENV EXPECTVER="Version : 1.0.0~0.1.b1+git.abcdefgh" \
	EXPECTREL="Release : 1"
RUN rpm -qpi /tmp/foo.rpm | sed -e 's/ \+/ /g' | grep "Version" > found.ver
RUN rpm -qpi /tmp/foo.rpm | sed -e 's/ \+/ /g' | grep "Release" > found.rel
RUN export FOUND_VER="$(cat found.ver)" && \
	echo "Expected: ${EXPECTVER}' :: Found: '${FOUND_VER}'" && \
	test "${FOUND_VER}" = "${EXPECTVER}"
RUN export FOUND_REL="$(cat found.rel)" && \
	echo "Expected: '${EXPECTREL}' :: Found: '${FOUND_REL}'" && \
	test "${FOUND_REL}" = "${EXPECTREL}"


# ---- changelog test ----
FROM min AS withchangelog
RUN rpm -qp /tmp/foo.rpm --changelog | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 1$"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 2$"
RUN rpm -qp /tmp/foo.rpm --changelog | grep -E "^- note 3$"
RUN rpm -q foo --changelog | grep "Carlos A Becker <pkg@carlosbecker.com>"
RUN rpm -q foo --changelog | grep -E "^- note 1$"
RUN rpm -q foo --changelog | grep -E "^- note 2$"
RUN rpm -q foo --changelog | grep -E "^- note 3$"


# ---- compression test ----
FROM min AS compression
ARG compression
RUN test "${compression}" = "$(rpm -qp --qf '%{PAYLOADCOMPRESSOR}' /tmp/foo.rpm)"
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/bin/fake

# ---- upgrade test ----
FROM test_base AS upgrade
ARG oldpackage
RUN echo "${oldpackage}"
COPY ${oldpackage} /tmp/old_foo.rpm
RUN rpm -ivh /tmp/old_foo.rpm

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Install"

RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Install"

RUN echo modified > /etc/regular.conf
RUN echo modified > /etc/noreplace.conf

RUN rpm -ivh /tmp/foo.rpm --upgrade

RUN test -f /tmp/preremove-proof
RUN cat /tmp/preremove-proof | grep "Upgrade"

RUN test -f /tmp/postremove-proof
RUN cat /tmp/postremove-proof | grep "Upgrade"

RUN test -f /tmp/preinstall-proof
RUN cat /tmp/preinstall-proof | grep "Upgrade"

RUN test -f /tmp/postinstall-proof
RUN cat /tmp/postinstall-proof | grep "Upgrade"

RUN cat /etc/regular.conf | grep foo=baz
RUN test -f /etc/regular.conf.rpmsave
RUN cat /etc/regular.conf.rpmsave | grep modified

RUN cat /etc/noreplace.conf | grep modified
RUN test -f /etc/noreplace.conf.rpmnew
RUN cat /etc/noreplace.conf.rpmnew | grep foo=baz

# ---- release test ----
FROM min AS release
RUN test -e /usr/bin/fake
RUN test -f /etc/foo/whatever.conf
RUN echo wat >> /etc/foo/whatever.conf
RUN rpm -e foo
RUN test -f /etc/foo/whatever.conf.rpmsave
RUN test ! -f /usr/bin/fake

# ---- directories test ----
FROM test_base AS directories
RUN groupadd test
RUN rpm -ivh /tmp/foo.rpm
RUN test -f /etc/foo/file
RUN test -f /etc/bar/file
RUN test -d /etc/bar
RUN test -d /etc/baz
RUN stat -L -c "%a %U %G" /etc/baz | grep "700 root test"
RUN rpm -e foo
RUN test ! -f /etc/foo/file
RUN test ! -f /etc/bar/file
RUN test -d /etc/foo
RUN test ! -d /etc/bar
RUN test ! -d /etc/baz
